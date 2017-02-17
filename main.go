package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	"net/http"

	"github.com/mitchellh/go-ps"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var procTotalMem = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: "node",
		Subsystem: "os",
		Name:      "process_memory",
		Help:      "Number of bytes allocated by process name (sum of all matching)",
	},
	[]string{"procname"})

func calculateMemory(pid int) (uint64, error) {
	f, err := os.Open(fmt.Sprintf("/proc/%d/smaps", pid))
	if err != nil {
		return 0, err
	}
	defer f.Close()

	res := uint64(0)
	pfx := []byte("Pss:")
	r := bufio.NewScanner(f)
	for r.Scan() {
		line := r.Bytes()
		if bytes.HasPrefix(line, pfx) {
			var size uint64
			_, err := fmt.Sscanf(string(line[4:]), "%d", &size)
			if err != nil {
				return 0, err
			}
			res += size
		}
	}
	if err := r.Err(); err != nil {
		return 0, err
	}

	return res, nil
}

func updateProcessCounters() {
	var procSizes = make(map[string]uint64)

	procs, err := ps.Processes()

	if err != nil {
		log.Print("Error fetching process list: ", err)
		return
	}

	for _, proc := range procs {
		size, err := calculateMemory(proc.Pid())
		if err == nil {
			procSizes[proc.Executable()] += size
		} else {
			log.Print("Error calculating memory for ", proc.Executable(), ": ",
				err)
		}
	}

	for name, size := range procSizes {
		procTotalMem.WithLabelValues(name).Set(float64(size))
	}
}

func processTicker(t <-chan time.Time) {
	updateProcessCounters()
	for range t {
		updateProcessCounters()
	}
}

func main() {
	var listen string
	var port int

	flag.StringVar(&listen, "listen", "",
		"Host name to bind to")
	flag.IntVar(&port, "port", 0,
		"Assign specific port to the listener")
	flag.Parse()

	prometheus.MustRegister(procTotalMem)

	go processTicker(time.Tick(1 * time.Minute))
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(
		net.JoinHostPort(listen, strconv.Itoa(port)), nil))
}
