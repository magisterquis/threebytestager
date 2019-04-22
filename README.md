Three-Byte Stager
=================
Serves up files, three bytes at a time, via DNS A records.  Meant to serve a
first-stage payload.  As transfers are high-overhead and really quite slow,
serving full binaries is not generally recommended if there is a faster way.

Sample stagers are in [stagers](./stagers/).  Please see the
[README](./stagers/README.md) in that directory for more information.

For legal use only.

Usage
-----
```sh
./threebytestager
```
By default this will serve DNS requests on `0.0.0.0:53` and serve files which
are in a directory named `./staged/`.  Please run with `-h` for configurable
options.

Protocol
--------
Requests should be of the form offset.filename.domain.  The offset is a 24-bit
hex number (i.e. from 0 to 0xFFFFFF).  The filename can be anything which
fits in a DNS label.  Responses are single A records consisting of a static
first byte (17) and three bytes from the file starting at the offset set in the
request.

In practical terms to request a file named `kmoused` using the domain
`example.com` the first request would be `ffffff.kmoused.example.com` to get
the file size, the next request would be `0.kmoused.example.com`, the request
after that `3.kmoused.example.com`, and so on up to the size returned by the
very first query.
