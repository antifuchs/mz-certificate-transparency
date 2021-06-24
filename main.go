package main

import (
	"fmt"
	"os"
	"time"

	"github.com/CaliDog/certstream-go"
	"github.com/jmoiron/jsonq"
	pubnub "github.com/pubnub/go"
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
	config := pubnub.NewConfig()
	config.SubscribeKey = os.Getenv("PN_SUBSCRIBE_KEY")
	config.PublishKey = os.Getenv("PN_PUBLISH_KEY")
	config.UUID = os.Getenv("PN_UUID")
	pn := pubnub.NewPubNub(config)

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
				logrus.WithFields(logrus.Fields{
					"message": cd,
				}).Debug("Received")
				_, _, err = pn.Publish().Channel("certstream").Message(cd).UsePost(true).Execute()
				if err != nil {
					logrus.WithError(err).Error("Could not publish")
					break Inner
				}
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
