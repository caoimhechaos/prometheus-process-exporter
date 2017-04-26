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
	"strings"
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
var procTotalCount = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: "node",
		Subsystem: "os",
		Name:      "num_processes",
		Help:      "Number of processes of a certain kind existing",
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
			// Size is in kB, so multiply by 1024.
			res += size * 1024
		}
	}
	if err := r.Err(); err != nil {
		return 0, err
	}

	return res, nil
}

func canonicalProcName(in string) string {
	if in[0] == '/' {
		var bestName string

		parts := strings.Split(in, "/")

		for _, curName := range parts {
			if len(bestName) == 0 {
				bestName = curName
				continue
			}

			// Ignore parts which are not descriptive of the
			// program itself.
			if curName != "usr" && curName != "bin" &&
				curName != "sbin" && curName != "local" {
				bestName = curName
			}
		}

		return bestName
	} else {
		// We know the first part is not empty, but there is a /
		// so this is probably something like "kthreadd/23".
		parts := strings.Split(in, "/")
		return parts[0]
	}
}

var procExists = make(map[string]bool)

func updateProcessCounters() {
	var procSizes = make(map[string]uint64)
	var procCounts = make(map[string]uint64)

	procs, err := ps.Processes()

	if err != nil {
		log.Print("Error fetching process list: ", err)
		return
	}

	for _, proc := range procs {
		procCounts[canonicalProcName(proc.Executable())]++
		size, err := calculateMemory(proc.Pid())
		if err == nil {
			procSizes[canonicalProcName(proc.Executable())] += size
		} else {
			log.Print("Error calculating memory for ", proc.Executable(), ": ",
				err)
		}
	}

	for name, size := range procSizes {
		procExists[name] = true
		procTotalMem.WithLabelValues(name).Set(float64(size))
	}
	for name, size := range procCounts {
		procExists[name] = true
		procTotalCount.WithLabelValues(name).Set(float64(size))
	}

	// Clean up dead data.
	for name, _ := range procExists {
		if _, ok := procSizes[name]; !ok {
			// Process disappeared, we need to clear it.
			procTotalMem.DeleteLabelValues(name)
			delete(procExists, name)
		}
		if _, ok := procCounts[name]; !ok {
			// Process disappeared, we need to clear it.
			procTotalCount.DeleteLabelValues(name)
			delete(procExists, name)
		}
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
	prometheus.MustRegister(procTotalCount)

	go processTicker(time.Tick(1 * time.Minute))
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(
		net.JoinHostPort(listen, strconv.Itoa(port)), nil))
}
