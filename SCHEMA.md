# Materialize view definitions for certificate transparency log sleuthing

## The source

We're reading from a kafka queue (and I'm going to postulate you'll
want to connect to a user/password-authenticated redpanda
instance). Adjust the broker, topic name, username/password and paste
this into your materialize connection:

```sql
CREATE MATERIALIZED SOURCE IF NOT EXISTS
certificate_transparency_src (json_data)
FROM KAFKA BROKER
-- this is one of your redpanda "cluster hosts":
'broker1:89273, broker2:89273'
TOPIC 'mz-hackday-demo'
WITH (
  security_protocol = 'sasl_ssl',
  client_id = 'materialize_cloud', -- could be anything
  sasl_mechanisms = 'SCRAM-SHA-256',

  -- Create a service accont and put in the account name & password here:
  sasl_username = 'REDACTED',
  sasl_password = 'REDACTED'
)
FORMAT BYTES;  -- we'll destructure the json later
```

### Intermission: Drop all these views for recreation

```sql
DROP VIEW IF EXISTS certificate_transparency_json CASCADE;
```

### Destructuring the bytes

What we get from the source above is a set of rows containing JSON
encoded as bytes. We want to use JSONB and from there, columns with
meaning. Let's make that happen:

```sql
CREATE VIEW IF NOT EXISTS certificate_transparency_json AS
SELECT CAST(json_data AS JSONB) AS json_data
FROM (
    SELECT CONVERT_FROM(json_data, 'utf8') AS json_data
    FROM certificate_transparency_src
);

CREATE VIEW IF NOT EXISTS certificate_transparency_log AS
SELECT
        (json_data->'domains') AS domains,
        (json_data->>'issuer_cn') AS issuer_cn,
        TIMESTAMP WITH TIME ZONE 'epoch' + (json_data->>'not_before')::bigint * INTERVAL '1 second' AS not_before_ts,
        TIMESTAMP WITH TIME ZONE 'epoch' + (json_data->>'not_after')::bigint * INTERVAL '1 second' AS not_after_ts,
        (json_data->>'not_before')::bigint * 1000 AS not_before_lts,
        (json_data->>'not_after')::bigint * 1000 AS not_after_lts,
        (json_data->>'serial_number') AS serial_number
FROM certificate_transparency_json;
```

We can watch a number go up (the number of ct log entries imported):

```sql
select count(*) from certificate_transparency_log;
```

Now we have a view, `certificate_transparency_log`, that contains the
destructured data in a form that we can process! Let's make use of it
to create some views:


### Doing some analysis part 1: Certs that are already expired

Now, we collect those certs that were expired at the time that we
picked them up from the log. We're also going to make a *materialized*
view, that is - one that will get automatically updated when new data
comes in.


Note that this uses logical timestamps - see
https://materialize.com/docs/sql/functions/now_and_mz_logical_timestamp/
for a summary of how that works.

``` sql
CREATE MATERIALIZED VIEW IF NOT EXISTS ct_expired_certs AS
SELECT
      domains,
      issuer_cn,
      not_after_ts
FROM certificate_transparency_log
WHERE
      mz_logical_timestamp() > not_after_lts;
```

You can now find expired certs with:

```sql
select * from ct_expired_certs;
```

### Doing some analysis part 2: Certs with "long" validity

A little while ago, there was a (pretty funny honestly) problem report
about Let's Encrypt certificates:
https://bugzilla.mozilla.org/show_bug.cgi?id=1715455 - they were
issued to be valid for exactly 90 days *plus one second*. This view
here will collect all certificates that are valid for longer than 90
days:

``` sql
CREATE MATERIALIZED VIEW IF NOT EXISTS ct_long_validity_certs AS
SELECT
      domains,
      issuer_cn,
      not_after_ts - not_before_ts AS validity
FROM certificate_transparency_log
WHERE
     not_after_ts - not_before_ts >= '90 days'::interval;
```

Now, let's look at the issuers that give out the longest-validity certs:

```sql
select max(EXTRACT(EPOCH FROM validity)) * '1 second'::interval, issuer_cn from ct_long_validity_certs group by 2 order by 1 desc limit 10;
```

Or, let's find those certificates that look like they may have the
same kind of inadvertently-weird validity:

```sql
SELECT
    max(EXTRACT(EPOCH FROM validity)) * '1 second'::interval as max_validity,
    issuer_cn from ct_long_validity_certs
WHERE
    validity > '90 days'::interval
    and validity < '91 days'::interval
GROUP BY 2
ORDER BY 1 DESC;
```
