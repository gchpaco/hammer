package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
)

var reportEvery = flag.Duration("i", 10*time.Second, "reporting interval")
var subReportEvery = flag.Int64("o", 100, "operations to perform before sub thread report")
var concurrency = flag.Int("c", 10, "# of concurrent requests")

func doRequest(url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return nil
}

type report struct {
	id       int64
	requests int64
	failures int64
	duration time.Duration
}

func (r *report) Add(other report) {
	r.requests += other.requests
	r.failures += other.failures
	r.duration += other.duration
}

func (r *report) FailPercent() float64 {
	return float64(r.failures) / float64(r.requests) * 100.0
}

func (r *report) AvgDuration() float64 {
	return float64(r.duration) / float64(r.requests)
}

func continuouslyRequest(id int64, url string, out chan<- report) {
	report := report{id: id}
	for {
		start := time.Now()
		err := doRequest(url)
		report.duration += time.Since(start)
		report.requests++
		if err != nil {
			log.Print(err)
			report.failures++
		}
		if (report.requests % *subReportEvery) == 0 {
			select {
			case out <- report:
				break
			default:
				log.Println("dropped?")
			}
		}
	}
}

func logReports(in <-chan report) {
	ticker := time.NewTicker(*reportEvery)
	defer ticker.Stop()
	reports := make(map[int64]report)

	for {
		select {
		case rpt := <-in:
			reports[rpt.id] = rpt
		case <-ticker.C:
			sum := &report{}
			for _, rpt := range reports {
				sum.Add(rpt)
			}
			log.Printf("Seen %d requests\t%2.2g%% errors\t%0.2gs response\n", sum.requests, sum.FailPercent(), sum.AvgDuration()/float64(time.Second))
		}
	}
}

func main() {
	flag.Parse()

	if flag.NArg() != 1 {
		log.Println("Usage: hammer [options] URL")
		flag.PrintDefaults()
		os.Exit(2)
	}

	url := flag.Arg(0)
	reports := make(chan report, *concurrency)

	for i := 0; i < *concurrency; i++ {
		fixed := i
		go continuouslyRequest(int64(fixed), url, reports)
	}

	logReports(reports)
}
