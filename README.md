# Fun times with certificate transparency logs and materialize!

This repo contains a little demo of what you can do to analyze my
favorite streaming distributed database: TLS certificate transparency
logs.

What are certificate transparency logs (ct logs, for short), you may
ask? Those are a cryptographically sealed and versioned set of
databases that are kept by various certificate authorities and browser
/ crawler vendors in order to detect maliciously or erroneously issued
certificates (the things that make HTTPS work). You can find more info
about them at https://certificate.transparency.dev/.

## So what do we do here?

This repo contains two things: A producer that reads the pre-processed
logs from https://certstream.calidog.io/ (a convenient way to access
the firehose that lets you get by without having to parse x509 data),
and feeds the resulting data into a kafka queue.

The other part is a set of materialized views, defined in
[SCHEMA.md](SCHEMA.md) that sets a
[materialize](https://materialize.com) instance up to use that data.
