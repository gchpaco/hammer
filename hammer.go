package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"time"
)

var reportEvery = flag.Duration("i", 10*time.Second, "reporting interval")
var subReportEvery = flag.Int64("o", 100, "operations to perform before sub thread report")
var concurrency = flag.Int("c", 10, "# of concurrent requests")

func doRequest(target string) error {
	resp, err := http.Get(target)
	if err != nil {
		if urlerr, ok := err.(*url.Error); ok {
			if urlerr.Err.Error() == "net/http: transport closed before response was received" {
				// this is sinfully ugly, I'm afraid, but in this case we retry
				// (once).  We have to match this way because the error in question
				// is unexported.
				resp, err = http.Get(target)
			}
		}
	}
	if err != nil {
		return fmt.Errorf("Error during GET: %s %s", err, reflect.ValueOf(err).Type())
	}
	defer resp.Body.Close()
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Error during body read: %s", err)
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
			fmt.Fprintf(os.Stdout, "Seen %12d requests %2.2g%% errors %0.2gs response\n", sum.requests, sum.FailPercent(), sum.AvgDuration()/float64(time.Second))
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
