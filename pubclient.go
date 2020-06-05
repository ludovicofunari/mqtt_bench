package main

import (
	"bytes"
	"fmt"
	"gonum.org/v1/gonum/stat/distuv"
	"log"
	"math"
	"os"
	"strings"

	"golang.org/x/exp/rand"
	//"math/rand"
	"strconv"
	"time"
)

import (
	"github.com/GaryBoone/GoStats/stats"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type PubClient struct {
	ID         string
	BrokerURL  string
	BrokerUser string
	BrokerPass string
	PubTopic   []int
	MsgSize    int
	MsgCount   int
	PubQoS     byte
	Quiet      bool
	//Users      int
	Lambda float64
}

func (c *PubClient) run(res chan *PubResults, ts chan int, distribution string, cv int) {
	newMsgs := make(chan *Message)
	pubMsgs := make(chan *Message)
	doneGen := make(chan bool)
	donePub := make(chan bool)
	runResults := new(PubResults)

	started := time.Now()
	// start generator
	go c.genMessages(newMsgs, doneGen)
	// start publisher
	go c.pubMessages(newMsgs, pubMsgs, doneGen, donePub, distribution, cv)

	runResults.ID = c.ID
	times := []float64{}
	for {
		select {
		case m := <-pubMsgs:
			if m.Error {
				log.Printf("Publisher-%v ERROR publishing message: %v: at %v\n", c.ID, m.Topic, m.Sent.Unix())
				runResults.Failures++
			} else {
				// log.Printf("Message published: %v: sent: %v delivered: %v flight time: %v\n", m.Topic, m.Sent, m.Delivered, m.Delivered.Sub(m.Sent))
				runResults.Successes++
				times = append(times, m.Delivered.Sub(m.Sent).Seconds()*1000) // in milliseconds
			}
		case <-donePub:
			// calculate results
			duration := time.Now().Sub(started)
			runResults.PubTimeMin = stats.StatsMin(times)
			runResults.PubTimeMax = stats.StatsMax(times)
			runResults.PubTimeMean = stats.StatsMean(times)
			runResults.PubTimeStd = stats.StatsSampleStandardDeviation(times)
			runResults.RunTime = duration.Seconds()
			runResults.PubsPerSec = float64(runResults.Successes) / duration.Seconds()

			// report results and exit
			res <- runResults
			return
		}
	}
}

func (c *PubClient) genMessages(ch chan *Message, done chan bool) {
	//var delay float64 = 1
	///r := rand.New(rand.NewSource(99))
	//r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < c.MsgCount; i++ {

		ch <- &Message{
			Topic: strconv.Itoa(c.PubTopic[0]),
			//Topic: "topic-" + strconv.Itoa(c.PubTopic[0]),
			//Topic: c.PubTopic,
			QoS: c.PubQoS,
			//Payload: make([]byte, c.MsgSize),
		}
	}
	done <- true
	log.Printf("PUBLISHER %v is done generating messages\n", c.ID)
	return
}

func (c *PubClient) pubMessages(in, out chan *Message, doneGen, donePub chan bool, distribution string, cv int) {
	onConnected := func(client mqtt.Client) {
		var delay float64

		ctr := 0

		for {
			select {
			case m := <-in:
				m.Sent = time.Now()
				convertedTime := strconv.FormatInt(m.Sent.UnixNano(), 10)
				m.Payload = bytes.Join([][]byte{[]byte(convertedTime), make([]byte, c.MsgSize)}, []byte("#@#"))

				// publish a message and increment msg counter
				token := client.Publish(m.Topic, m.QoS, false, m.Payload)
				token.Wait()
				out <- m
				ctr++

				if token.Error() != nil {
					log.Printf("Publisher-%v Error sending message: %v\n", c.ID, token.Error())
					m.Error = true
				} else {
					m.Delivered = time.Now()
					m.Error = false
				}

				if strings.ToLower(distribution) == "poisson" {
					// for poisson distribution
					r := rand.New(rand.NewSource(uint64(time.Now().UnixNano())))
					delay = math.Max(r.ExpFloat64()/c.Lambda-float64(m.Delivered.Sub(m.Sent).Seconds()), 0)
				} else if strings.ToLower(distribution) == "lognormal" {
					// for lognormal distribution
					Ti := 1 / c.Lambda
					v := math.Pow(float64(cv)*Ti, 2)
					mu := math.Log(math.Pow(Ti, 2) / math.Sqrt(v+math.Pow(Ti, 2)))
					sigma := math.Sqrt(math.Log((v / math.Pow(Ti, 2)) + 1))
					src := rand.New(rand.NewSource(uint64(time.Now().UnixNano())))
					delay = math.Max(distuv.LogNormal{Mu: mu, Sigma: sigma, Src: src}.Rand()-float64(m.Delivered.Sub(m.Sent).Seconds()), 0)
				} else {
					log.Println("Typed wrong distribution as argument. Exiting...")
					os.Exit(1)
				}

				// wait for next msg publication
				time.Sleep(time.Duration(delay*1000000) * time.Microsecond)

			case <-doneGen:
				if !c.Quiet {
					log.Printf("Publisher-%v connected to broker %v, published on topic: %v\n", c.ID, c.BrokerURL, c.PubTopic)
				}
				donePub <- true
				client.Disconnect(250)
				return
			}
		}
	}

	opts := mqtt.NewClientOptions().
		AddBroker(c.BrokerURL).
		SetClientID(fmt.Sprintf("pub-%v", c.ID)).
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetOnConnectHandler(onConnected).
		SetConnectionLostHandler(func(client mqtt.Client, reason error) {
			log.Printf("Publisher-%v lost connection to the broker: %v. Will reconnect...\n", c.ID, reason.Error())
		})
	if c.BrokerUser != "" && c.BrokerPass != "" {
		opts.SetUsername(c.BrokerUser)
		opts.SetPassword(c.BrokerPass)
	}
	client := mqtt.NewClient(opts)
	token := client.Connect()
	token.Wait()

	if token.Error() != nil {
		log.Printf("Publisher-%v had error connecting to the broker: %v. Error: %v\n", c.ID, c.BrokerURL, token.Error())
	}
}
