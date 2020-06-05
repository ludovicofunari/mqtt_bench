package main

import (
	"bytes"
	"strconv"
	//"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/GaryBoone/GoStats/stats"
	"log"
	"math"
	"time"
)

// Message describes a message
type Message struct {
	Topic     string
	QoS       byte
	Payload   interface{}
	Sent      time.Time
	Delivered time.Time
	Error     bool
}

// SubResults describes results of a single SUBSCRIBER / run
type SubResults struct {
	ID             string  `json:"id"`
	Published      int64   `json:"actual_published"`
	Received       int64   `json:"received"`
	FwdRatio       float64 `json:"fwd_success_ratio"`
	FwdLatencyMin  float64 `json:"fwd_time_min"`
	FwdLatencyMax  float64 `json:"fwd_time_max"`
	FwdLatencyMean float64 `json:"fwd_time_mean"`
	FwdLatencyStd  float64 `json:"fwd_time_std"`
	SubsPerSec     float64 `json:"sub_per_sec"`
	Duration       float64 `json:"duration"`
	AvgMsgsPerSec  float64 `json:"avg_msgs_per_sec"`
}

// TotalSubResults describes results of all SUBSCRIBER / runs
type TotalSubResults struct {
	TotalFwdRatio     float64 `json:"fwd_success_ratio"`
	TotalReceived     int64   `json:"successes"`
	TotalPublished    int64   `json:"actual_total_published"`
	FwdLatencyMin     float64 `json:"fwd_latency_min"`
	FwdLatencyMax     float64 `json:"fwd_latency_max"`
	FwdLatencyMeanAvg float64 `json:"fwd_latency_mean_avg"`
	FwdLatencyMeanStd float64 `json:"fwd_latency_mean_std"`
	TotalMsgsPerSec   float64 `json:"avg_msgs_per_sec"`
}

// PubResults describes results of a single PUBLISHER / run
type PubResults struct {
	ID          string  `json:"id"`
	Successes   int64   `json:"pub_successes"`
	Failures    int64   `json:"failures"`
	RunTime     float64 `json:"run_time"`
	PubTimeMin  float64 `json:"pub_time_min"`
	PubTimeMax  float64 `json:"pub_time_max"`
	PubTimeMean float64 `json:"pub_time_mean"`
	PubTimeStd  float64 `json:"pub_time_std"`
	PubsPerSec  float64 `json:"publish_per_sec"`
}

// TotalPubResults describes results of all PUBLISHER / runs
type TotalPubResults struct {
	PubRatio        float64 `json:"publish_success_ratio"`
	Successes       int64   `json:"successes"`
	Failures        int64   `json:"failures"`
	TotalRunTime    float64 `json:"total_run_time"`
	AvgRunTime      float64 `json:"avg_run_time"`
	PubTimeMin      float64 `json:"pub_time_min"`
	PubTimeMax      float64 `json:"pub_time_max"`
	PubTimeMeanAvg  float64 `json:"pub_time_mean_avg"`
	PubTimeMeanStd  float64 `json:"pub_time_mean_std"`
	TotalMsgsPerSec float64 `json:"total_msgs_per_sec"`
	AvgMsgsPerSec   float64 `json:"avg_msgs_per_sec"`
}

// JSONResults are used to export results as a JSON document
type JSONResults struct {
	PubRuns   []*PubResults    `json:"publish runs"`
	SubRuns   []*SubResults    `json:"subscribe runs"`
	PubTotals *TotalPubResults `json:"publish totals"`
	SubTotals *TotalSubResults `json:"receive totals"`
}

func main() {

	var (
		size         = flag.Int("size", 100, "Size of the messages payload (bytes).")
		username     = flag.String("username", "", "MQTT username (empty if auth disabled)")
		password     = flag.String("password", "", "MQTT password (empty if auth disabled)")
		pubqos       = flag.Int("pubqos", 0, "QoS for published messages, default is 0")
		subqos       = flag.Int("subqos", 0, "QoS for subscribed messages, default is 0")
		count        = flag.Int("count", 1, "Number of messages to send per pubclient.")
		quiet        = flag.Bool("quiet", false, "Suppress logs while running, default is false")
		lambda       = flag.Float64("pubrate", 1.0, "Publishing exponential rate (msg/sec).")
		file         = flag.String("file", "test.json", "Import subscribers, publishers and topic information from file.")
		nodeport     = flag.Int("nodeport", 30123, "Kubernetes NodepPort for VerneMQ MQTT service.")
		distribution = flag.String("dist", "poisson", "Select Poisson or Lognormal distribution (default Poisson)")
		cv           = flag.Int("cv", 4, "Select coefficient of variation for the Lognormal distribution (default 4)")
	)

	flag.Parse()

	format := "text"

	var user Users
	var arraySubTopics []map[string]byte
	nodeIDs := make(map[int]string)

	user, arraySubTopics, nodeIDs = populateFromFile(*file, *nodeport)

	//start subscribe
	subResCh := make(chan *SubResults)
	jobDone := make(chan bool)
	subDone := make(chan bool)
	subCnt := 0

	if !*quiet {
		log.Printf("Starting to subscribe...\n")
	}

	for i := 0; i < len(user.Subscribers); i++ {
		sub := &SubClient{
			ID: strconv.FormatFloat(user.Subscribers[i].SubID, 'f', -1, 64),
			//BrokerURL:  "tcp://localhost:1883",
			BrokerURL:  nodeIDs[user.Subscribers[i].NodeID],
			BrokerUser: *username,
			BrokerPass: *password,
			SubTopic:   arraySubTopics[i],
			SubQoS:     byte(*subqos),
			Quiet:      *quiet,
			Count:      *count,
		}
		go sub.run(subResCh, subDone, jobDone)
	}

SUBJOBDONE:
	for {
		select {
		case <-subDone:
			subCnt++
			if subCnt == len(user.Subscribers) {
				if !*quiet {
					log.Printf("All subscribtion jobs are done.\n")
				}
				break SUBJOBDONE
			}
		}
	}

	//time.Sleep(3600 * time.Second)
	//log.Println("Time is up.")

	//start publish
	if !*quiet {
		log.Printf("Starting publish...\n")
	}
	pubResCh := make(chan *PubResults)
	timeSeq := make(chan int)

	start := time.Now()
	for i := 0; i < len(user.Publishers); i++ {
		c := &PubClient{
			ID: strconv.FormatFloat(user.Publishers[i].PubID, 'f', -1, 64),
			//BrokerURL:  "tcp://localhost:1883",
			BrokerURL:  nodeIDs[user.Publishers[i].NodeID],
			BrokerUser: *username,
			BrokerPass: *password,
			PubTopic:   user.Publishers[i].TopicList,
			MsgSize:    *size,
			MsgCount:   *count,
			PubQoS:     byte(*pubqos),
			Quiet:      *quiet,
			Lambda:     *lambda,
		}
		go c.run(pubResCh, timeSeq, *distribution, *cv)
	}

	// collect the publish results
	pubresults := make([]*PubResults, len(user.Publishers))

	for i := 0; i < len(user.Publishers); i++ {
		pubresults[i] = <-pubResCh
	}

	totalTime := time.Now().Sub(start)
	pubtotals := calculatePublishResults(pubresults, totalTime)

	for i := 0; i < 3; i++ {
		time.Sleep(1 * time.Second)
		if !*quiet {
			log.Printf("Benchmark will stop after %v seconds.\n", 3-i)
		}
	}

	// notify subscriber that job done
	for i := 0; i < len(user.Subscribers); i++ {
		jobDone <- true
	}

	// collect subscribe results
	subresults := make([]*SubResults, len(user.Subscribers))
	for i := 0; i < len(user.Subscribers); i++ {
		subresults[i] = <-subResCh
	}

	// collect the sub results
	subtotals := calculateSubscribeResults(subresults, pubresults)

	// print stats
	printResults(pubresults, pubtotals, subresults, subtotals, format)

	fmt.Printf("All jobs done. Time spent for the benchmark: %vs\n", math.Round(float64(*count) / *lambda))
	fmt.Println("======================================================")
}

func calculatePublishResults(pubresults []*PubResults, totalTime time.Duration) *TotalPubResults {
	pubtotals := new(TotalPubResults)
	pubtotals.TotalRunTime = totalTime.Seconds()

	pubTimeMeans := make([]float64, len(pubresults))
	msgsPerSecs := make([]float64, len(pubresults))
	runTimes := make([]float64, len(pubresults))
	bws := make([]float64, len(pubresults))

	pubtotals.PubTimeMin = pubresults[0].PubTimeMin
	for i, res := range pubresults {
		pubtotals.Successes += res.Successes
		pubtotals.Failures += res.Failures
		pubtotals.TotalMsgsPerSec += res.PubsPerSec

		if res.PubTimeMin < pubtotals.PubTimeMin {
			pubtotals.PubTimeMin = res.PubTimeMin
		}

		if res.PubTimeMax > pubtotals.PubTimeMax {
			pubtotals.PubTimeMax = res.PubTimeMax
		}

		pubTimeMeans[i] = res.PubTimeMean
		msgsPerSecs[i] = res.PubsPerSec
		runTimes[i] = res.RunTime
		bws[i] = res.PubsPerSec
	}
	pubtotals.PubRatio = float64(pubtotals.Successes) / float64(pubtotals.Successes+pubtotals.Failures)
	pubtotals.AvgMsgsPerSec = stats.StatsMean(msgsPerSecs)
	pubtotals.AvgRunTime = stats.StatsMean(runTimes)
	pubtotals.PubTimeMeanAvg = stats.StatsMean(pubTimeMeans)
	pubtotals.PubTimeMeanStd = stats.StatsSampleStandardDeviation(pubTimeMeans)

	return pubtotals
}

func calculateSubscribeResults(subresults []*SubResults, pubresults []*PubResults) *TotalSubResults {
	subtotals := new(TotalSubResults)
	fwdLatencyMeans := make([]float64, len(subresults))
	msgPerSec := make([]float64, len(subresults))

	subtotals.FwdLatencyMin = subresults[0].FwdLatencyMin
	for i, res := range subresults {
		subtotals.TotalReceived += res.Received

		if res.FwdLatencyMin < subtotals.FwdLatencyMin {
			subtotals.FwdLatencyMin = res.FwdLatencyMin
		}

		if res.FwdLatencyMax > subtotals.FwdLatencyMax {
			subtotals.FwdLatencyMax = res.FwdLatencyMax
		}

		fwdLatencyMeans[i] = res.FwdLatencyMean
		for _, pubres := range pubresults {
			if pubres.ID == res.ID {
				subtotals.TotalPublished += pubres.Successes
				res.Published = pubres.Successes
				res.FwdRatio = float64(res.Received) / float64(pubres.Successes)
			}
		}
		msgPerSec[i] = res.AvgMsgsPerSec
		subtotals.TotalMsgsPerSec += msgPerSec[i]
	}
	subtotals.FwdLatencyMeanAvg = stats.StatsMean(fwdLatencyMeans)
	subtotals.FwdLatencyMeanStd = stats.StatsSampleStandardDeviation(fwdLatencyMeans)
	subtotals.TotalFwdRatio = float64(subtotals.TotalReceived) / float64(subtotals.TotalPublished)
	//subtotals.TotalMsgsPerSec += msgPerSec
	return subtotals
}

func printResults(pubresults []*PubResults, pubtotals *TotalPubResults, subresults []*SubResults, subtotals *TotalSubResults, format string) {
	switch format {
	case "json":
		jr := JSONResults{
			PubRuns:   pubresults,
			SubRuns:   subresults,
			PubTotals: pubtotals,
			SubTotals: subtotals,
		}
		data, _ := json.Marshal(jr)
		var out bytes.Buffer
		_ = json.Indent(&out, data, "", "\t")

		fmt.Println(string(out.Bytes()))
	default:
		// TODO: specify distribution
		//fmt.Printf("\nPublished using a %v distribution ")
		fmt.Printf("\n")
		fmt.Printf("================= TOTAL PUBLISHER (%d) =================\n", len(pubresults))
		fmt.Printf("Total Publish Success Ratio:   %.2f%% (%d/%d)\n", pubtotals.PubRatio*100, pubtotals.Successes, pubtotals.Successes+pubtotals.Failures)
		fmt.Printf("Average Runtime (sec):         %.2f\n", pubtotals.AvgRunTime)
		fmt.Printf("Pub time min (ms):             %.2f\n", pubtotals.PubTimeMin)
		fmt.Printf("Pub time max (ms):             %.2f\n", pubtotals.PubTimeMax)
		fmt.Printf("Pub time mean mean (ms):       %.2f\n", pubtotals.PubTimeMeanAvg)
		fmt.Printf("Pub time mean std (ms):        %.2f\n", pubtotals.PubTimeMeanStd)
		fmt.Printf("Average Bandwidth (msg/sec):   %.2f\n", pubtotals.AvgMsgsPerSec)
		fmt.Printf("Total Bandwidth (msg/sec):     %.2f\n\n", pubtotals.TotalMsgsPerSec)

		fmt.Printf("================= TOTAL SUBSCRIBER (%d) =================\n", len(subresults))
		fmt.Printf("Total Forward Success Ratio:      %.2f%% (%d/%d)\n", subtotals.TotalFwdRatio*100, subtotals.TotalReceived, subtotals.TotalPublished)
		fmt.Printf("Forward latency min (ms):         %.2f\n", subtotals.FwdLatencyMin)
		fmt.Printf("Forward latency max (ms):         %.2f\n", subtotals.FwdLatencyMax)
		fmt.Printf("Forward latency mean std (ms):    %.2f\n", subtotals.FwdLatencyMeanStd)
		fmt.Printf("Total Mean forward latency (ms):  %.2f\n\n", subtotals.FwdLatencyMeanAvg)

		fmt.Printf("Total Receiving rate (msg/sec): %.2f\n", subtotals.TotalMsgsPerSec)
	}
	return
}
