package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/CaliDog/certstream-go"
	"github.com/etherlabsio/healthcheck/v2"
	"github.com/gorilla/mux"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/jmoiron/jsonq"
	"github.com/sirupsen/logrus"
)

// CertData holds information about the issued leaf certificate and
// the certificate that issued it (but no information about the trust
// chain leading up to the root).
type CertData struct {
	Domains      []string `json:"domains"`
	NotBefore    int      `json:"not_before"`
	NotAfter     int      `json:"not_after"`
	SerialNumber string   `json:"serial_number"`
	Fingerprint  string   `json:"fingerprint"`
	IssuerCN     string   `json:"issuer_cn"`
}

func parseCertUpdate(msg jsonq.JsonQuery) (cd CertData, err error) {
	var msgType string
	msgType, err = msg.String("message_type")
	if err != nil {
		return
	}
	if msgType != "certificate_update" {
		err = fmt.Errorf("Encountered unknown message type %v", msgType)
		return
	}

	cd.Domains, err = msg.ArrayOfStrings("data", "leaf_cert", "all_domains")
	if err != nil {
		return
	}

	cd.NotBefore, err = msg.Int("data", "leaf_cert", "not_before")
	if err != nil {
		return
	}

	cd.NotAfter, err = msg.Int("data", "leaf_cert", "not_after")
	if err != nil {
		return
	}

	cd.SerialNumber, err = msg.String("data", "leaf_cert", "serial_number")
	if err != nil {
		return
	}

	cd.IssuerCN, err = msg.String("data", "leaf_cert", "issuer", "aggregated")
	if err != nil {
		return
	}
	return
}

func main() {
	// logrus.SetLevel(logrus.DebugLevel)
	logrus.SetLevel(logrus.InfoLevel)
	logrus.WithField("env", os.Environ()).Debug("Starting up")

	producer, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": os.Getenv("KAFKA_BROKER"),
		"client.id":         os.Getenv("KAFKA_CLIENTID"),
		"acks":              "all",
		"security.protocol": "sasl_ssl",
		"sasl.mechanism":    "SCRAM-SHA-256",
		"sasl.username":     os.Getenv("KAFKA_USER"),
		"sasl.password":     os.Getenv("KAFKA_PASSWORD"),
	})
	if err != nil {
		logrus.WithError(err).Fatal("couldn't connect to kafka")
	}
	deliveryChan := make(chan kafka.Event, 10000)
	topic_name := os.Getenv("KAFKA_TOPIC")
	partition := kafka.TopicPartition{Topic: &topic_name, Partition: kafka.PartitionAny}

	// Health checks:
	var lastRead time.Time
	var lastSent time.Time
	r := mux.NewRouter()
	r.Handle("/healthcheck", healthcheck.Handler(
		// WithTimeout allows you to set a max overall timeout.
		healthcheck.WithTimeout(5*time.Second),

		healthcheck.WithChecker(
			"read", healthcheck.CheckerFunc(
				func(ctx context.Context) error {
					if time.Now().Sub(lastRead) > 10*time.Minute {
						return fmt.Errorf("Last read was at %v, more than 10min ago", lastRead)
					}
					return nil
				},
			)),
		healthcheck.WithChecker(
			"sent", healthcheck.CheckerFunc(
				func(ctx context.Context) error {
					if time.Now().Sub(lastSent) > 10*time.Minute {
						return fmt.Errorf("Last send was at %v, more than 10min ago", lastRead)
					}
					return nil
				},
			)),
	))
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	go func() {
		http.ListenAndServe(":"+port, r)
	}()

	go func() {
		for e := range producer.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				if ev.TopicPartition.Error != nil {
					logrus.WithError(ev.TopicPartition.Error).WithField("partition", ev.TopicPartition).Warn("Failed to deliver message")
				}
			}
		}
	}()

	for {
		certs, errStream := certstream.CertStreamEventStream(false)
		maxLifetime := time.After(10 * time.Minute)
	Inner:
		for {
			select {
			case jq := <-certs:
				cd, err := parseCertUpdate(jq)
				if err != nil {
					logrus.WithError(err).Warn("Error decoding certstream event")
					continue Inner
				}
				lastRead = time.Now()

				logrus.WithFields(logrus.Fields{
					"message": cd,
				}).Debug("Received")
				cdB, err := json.Marshal(cd)
				if err != nil {
					logrus.WithError(err).Error("Couldn't serialize")
					break Inner
				}
				producer.Produce(&kafka.Message{
					TopicPartition: partition,
					Value:          cdB,
				}, deliveryChan)
				lastSent = time.Now()
			case err := <-errStream:
				logrus.WithError(err).Error("certstream receiver received error")
				break Inner
			case <-maxLifetime:
				// close channels, restart:
				logrus.Info("Restarting read after 10min have elapsed")
				close(certs)
				close(errStream)
				break Inner
			}
		}
	}
}
