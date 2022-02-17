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


### Running this yourself

You need two things: A materialize instance (I, cough, recommend
https://cloud.materialize.com) and a kafka topic (you can get one at
https://redpanda.com).

Then you configure the producer to feed data into the kafka topic, and
you connect to the materialize instance to setup the schema (see
[SCHEMA.md](SCHEMA.md)).

#### Setting up the kafka queue

Ideally, use a broker configured for SASL PLAINTEXT auth over SSL. Set
up one kafka topic, two accounts with application-specific passwords
(or "service accounts").

One service account should have write access to your topic (in the
example, that's the `mz-hackday-demo-producer` account), and the other
should have read access (use that account for the `CREATE SOURCE`
credentials in SCHEMA.md).

#### Running the producer

The producer takes several environment variables. Here's a shell
snippet you can adjust for your values:

```sh
export KAFKA_USER="mz-hackday-demo"
export KAFKA_PASSWORD="your_secure_password"
export KAFKA_BROKER="broker1:30423"
export KAFKA_CLIENTID="mz-hackday-demo-producer"
export KAFKA_TOPIC="mz-hackday-demo"
```

Export those environment variables and run `go run ./`. It should
start up and feed data into your kafka queue!
