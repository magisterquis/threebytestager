/* Program threebytestager serves up a staged file via DNS */
package main

/*
 * threebytestager.go
 * Serves files, theree bytes at a time
 * By J. Stuart McMurray
 * Created 20190329
 * Last Modified 20190422
 */

import (
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/net/dns/dnsmessage"
)

type file struct {
	size     [4]byte /* File size as an A record */
	contents []byte  /* Contents of file */
}

var (
	/* pool is the packet buffer pool */
	pool = &sync.Pool{New: func() interface{} { return make([]byte, 1024) }}

	/* Files to serve */
	files  = make(map[string]file)
	filesL sync.RWMutex

	/* errorResource is the A resource body to return on error */
	errorResource = dnsmessage.AResource{A: [4]byte{0, 0, 0, 0}}
)

func main() {
	var (
		dir = flag.String(
			"file-dir",
			"staged",
			"Name of `directory` containing files to stage",
		)
		laddr = flag.String(
			"listen",
			"0.0.0.0:53",
			"Listen `address` for DNS service",
		)
		firstByte = flag.Uint(
			"first-octet",
			17,
			"First `octet` to set in A records",
		)
		ttl = flag.Uint64(
			"ttl",
			300,
			"Response time to live, in `seconds`",
		)
	)
	flag.Usage = func() {
		fmt.Fprintf(
			os.Stderr,
			`Usage: %v [options]

Serves up files over DNS, three bytes at a time.  A request for offset 0xFFFFFF
of a file will return the file size, which effectively limits a file to
16,777,214 bytes.

Requests should be of the form offset.filename.domain.  The offset should be a
hex number from 0x0 to 0xFFFFFF.

Options:
`,
			os.Args[0],
		)
		flag.PrintDefaults()
	}
	flag.Parse()

	/* Make sure our first octet is an octet */
	if 0xFF < *firstByte {
		log.Fatalf("First octet must be <= 255")
	}
	fb := byte(*firstByte)
	errorResource.A[0] = fb

	/* Make sure the TTL isn't too much */
	if math.MaxUint32 < *ttl {
		log.Fatalf("TTL is too large")
	}

	/* Listen for DNS requests */
	pc, err := net.ListenPacket("udp", *laddr)
	if nil != err {
		log.Fatalf("Unable to listen on %v: %v", *laddr, err)
	}
	log.Printf("Will serve DNS queries on %v", pc.LocalAddr())

	/* Read packets, reply */
	for {
		/* Pop a packet */
		buf := pool.Get().([]byte)
		n, addr, err := pc.ReadFrom(buf)
		if nil != err {
			log.Fatalf("ReadFrom: %v", err)
		}
		/* Handle it */
		go func() {
			go handle(pc, addr, buf[:n], *dir, uint32(*ttl), fb)
			pool.Put(buf)
		}()
	}
}

/* handle get the bytes requested.  If a file's not been read, it gets read */
func handle(
	pc net.PacketConn,
	addr net.Addr,
	qbuf []byte,
	dir string,
	ttl uint32,
	fb byte, /* First byte in replies */
) {
	/* Answer resource */
	var a dnsmessage.AResource
	a.A = errorResource.A

	/* Unmarshal packet */
	var m dnsmessage.Message
	if err := m.Unpack(qbuf); nil != err {
		log.Printf("[%v] Invalid packet: %v", addr, err)
		return
	}

	/* Make sure we have at least one question */
	if 0 == len(m.Questions) {
		log.Printf("[%v] No questions", addr)
		return
	}

	tag := fmt.Sprintf("[%v (%v)]", m.Questions[0].Name, addr)

	/* Make sure a reply is sent */
	defer func() {
		var err error
		/* Make sure we know we're sending a response */
		m.Header.Response = true

		/* Add in the answer */
		m.Answers = append(m.Answers, dnsmessage.Resource{
			Header: dnsmessage.ResourceHeader{
				Name:  m.Questions[0].Name,
				Type:  dnsmessage.TypeA,
				Class: dnsmessage.ClassINET,
				TTL:   ttl,
			},
			Body: &a,
		})

		/* Roll the reply */
		rbuf := pool.Get().([]byte)
		defer pool.Put(rbuf)
		if rbuf, err = m.AppendPack(rbuf[:0]); nil != err {
			log.Printf("%v Unable to roll reply: %v", tag, err)
			return
		}

		/* Send it back */
		if _, err := pc.WriteTo(rbuf, addr); nil != err {
			log.Printf("%v Error sending reply: %v", tag, err)
			return
		}

		/* Only log if we sent something meaningful */
		if a.A != errorResource.A {
			log.Printf(
				"%v %v",
				tag,
				net.IP(m.Answers[0].Body.(*dnsmessage.AResource).A[:]),
			)
		}
	}()

	/* Get the offset and file name */
	parts := strings.SplitN(m.Questions[0].Name.String(), ".", 3)
	if 3 != len(parts) {
		log.Printf("%v Not enough labels", tag)
		return
	}

	pu, err := strconv.ParseUint(parts[0], 16, 32)
	offset := uint32(pu)
	if nil != err {
		log.Printf("%v Unparsable offset %q: %v", tag, parts[0], err)
		return
	}
	fname := strings.ToLower(parts[1])

	/* Make sure we have this file */
	if err := ensureFile(dir, fname, fb); nil != err {
		log.Printf("%v Unpossible file %q", tag, fname)
		return
	}

	/* Get the chunk or the file size */
	if 0xFFFFFF == offset {
		/* Request for size */
		a.A = getSize(fname)
	} else {
		var ok bool
		a.A, ok = getOffset(fname, offset, fb)
		if !ok {
			log.Printf("%v Too-large offset %v", tag, offset)
			return
		}
	}
}

/* ensureFile tries to ensure the file named n is in the map.  If it's not able
to be put there, it returns an error. */
func ensureFile(dir, n string, fb byte) error {
	filesL.Lock()
	defer filesL.Unlock()

	/* If we have it, we're all set */
	if _, ok := files[n]; ok {
		return nil
	}

	/* If not, try to open it */
	buf, err := ioutil.ReadFile(filepath.Join(dir, n))
	if nil != err {
		return err
	}
	if 0 == len(buf) {
		return errors.New("empty file")
	}

	/* Get the file size */
	if math.MaxUint32 < len(buf) {
		return errors.New("file way too large")
	}
	var a [4]byte
	binary.BigEndian.PutUint32(a[:], uint32(len(buf)))
	if 0 != a[0] {
		return errors.New("file too large")
	}
	a[0] = fb

	/* TODO: Some sort of caching */

	files[n] = file{a, buf}
	log.Printf("New file: %v", n)

	return nil
}

/* getSize gets the size of a file named n */
func getSize(n string) [4]byte {
	filesL.RLock()
	defer filesL.RUnlock()

	/* Get hold of the file */
	f, ok := files[n]
	if !ok {
		log.Panicf("no file %q for size", n)
	}

	/* Get the file size */
	var a [4]byte
	a = f.size
	return a
}

/* getOffset gets the 3 bytes at the given offset.  The first byte of the
returned array is always fb. */
func getOffset(n string, offset uint32, fb byte) ([4]byte, bool) {
	filesL.RLock()
	defer filesL.RUnlock()

	/* Get hold of the file */
	f, ok := files[n]
	if !ok {
		log.Panicf("no file %q for offset", n)
	}

	/* Make sure we have enough file */
	var a [4]byte
	if uint32(len(f.contents)-1) < offset {
		return a, false
	}
	a[0] = fb
	copy(a[1:], f.contents[offset:])
	return a, true
}
