package main

import (
	"fmt"
	"log"
	"strconv"
	"time"
)

import (
	"github.com/GaryBoone/GoStats/stats"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type SubClient struct {
	ID         string
	BrokerURL  string
	BrokerUser string
	BrokerPass string
	SubTopic   map[string]byte
	SubQoS     byte
	Quiet      bool
	Count      int
	FirstTime  float64
	LastTime   float64
}

func (c *SubClient) run(res chan *SubResults, subDone chan bool, jobDone chan bool) {
	runResults := new(SubResults)
	runResults.ID = c.ID
	c.FirstTime = 0
	c.LastTime = 0

	var forwardLatency []float64

	opts := mqtt.NewClientOptions().
		AddBroker(c.BrokerURL).
		SetClientID(fmt.Sprintf("sub-%v", c.ID)).
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
			recvTime := time.Now().UnixNano()
			if c.FirstTime == 0 {
				c.FirstTime = float64(recvTime)
			}
			c.LastTime = float64(recvTime)
			//started := time.Now()
			payload := msg.Payload()
			i := 0
			for ; i < len(payload)-3; i++ {
				if payload[i] == '#' && payload[i+1] == '@' && payload[i+2] == '#' {
					sendTime, _ := strconv.ParseInt(string(payload[:i]), 10, 64)
					forwardLatency = append(forwardLatency, float64((recvTime-sendTime)/1000000)) // in milliseconds
					break
				}
			}
			runResults.Received++
			//rate:= float64(runResults.Received)/((c.LastTime-c.FirstTime)/1e9)
			// log.Printf("SUBSCRIBER-%v, receiving rate %v \n", c.ID, rate)
			//runResults.Duration += time.Now().Sub(started).Seconds()

		}).
		SetConnectionLostHandler(func(client mqtt.Client, reason error) {
			log.Printf("Subscriber-%v lost connection to the broker: %v. Will reconnect...\n", c.ID, reason.Error())
		})

	//runResults.SubsPerSec = float64(runResults.Received) / duration.Seconds()

	if c.BrokerUser != "" && c.BrokerPass != "" {
		opts.SetUsername(c.BrokerUser)
		opts.SetPassword(c.BrokerPass)
	}
	client := mqtt.NewClient(opts)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Printf("Subscriber-%v had error connecting to the broker: %v\n", c.ID, token.Error())
		return
	}

	//if token := client.Subscribe("topic-" + strconv.Itoa(c.SubTopic[0]), c.SubQoS, nil); token.Wait() && token.Error() != nil {
	if token := client.SubscribeMultiple(c.SubTopic, nil); token.Wait() && token.Error() != nil {
		log.Printf("Subscriber-%v had error in subscribing to topics. Error: %v\n", c.ID, token.Error())
		//log.Printf("Subscriber-%v had error subscribe with topic: %v. Error: %v\n", c.ID, c.SubTopic, token.Error())
		fmt.Println(c.SubTopic)
		return
	}

	if !c.Quiet {
		log.Printf("Subscriber-%v connected to broker: %v\n", c.ID, c.BrokerURL)
	}

	subDone <- true
	//加各项统计
	for {
		select {
		case <-jobDone:
			client.Disconnect(250)
			runResults.FwdLatencyMin = stats.StatsMin(forwardLatency)
			runResults.FwdLatencyMax = stats.StatsMax(forwardLatency)
			runResults.FwdLatencyMean = stats.StatsMean(forwardLatency)
			runResults.FwdLatencyStd = stats.StatsSampleStandardDeviation(forwardLatency)
			runResults.AvgMsgsPerSec = float64(runResults.Received) / ((c.LastTime - c.FirstTime) / 1e9)
			//log.Printf("Subscriber-%v, receiving rate %v \n", c.ID, runResults.AvgMsgsPerSec)
			res <- runResults
			//if !c.Quiet {
			//	log.Printf("Subscriber-%v is done subscribing\n", c.ID)
			//}
			return
		}
	}
}
