package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HostStatus holds the real-time metrics for a single host.
type HostStatus struct {
	Host       string    `json:"host"`
	Status     string    `json:"status"` // "UP" or "DOWN"
	LatencyMs  float64   `json:"latencyMs"`
	PacketLoss float64   `json:"packetLoss"` // Percentage
	LastCheck  time.Time `json:"lastCheck"`
	CheckCount int       `json:"checkCount"`
}

// Global state protected by a RWMutex
var (
	hostStatuses = make(map[string]HostStatus)
	mu           sync.RWMutex
)

// Command line flags
var (
	hostsStr   string
	port       int
	intervalMs int
)

func init() {
	// Initialize command line flags
	flag.StringVar(&hostsStr, "hosts", "actiontarget.com, ksl.com, github.com", "Comma-separated list of hosts to monitor")
	flag.IntVar(&port, "port", 8080, "Port for the web dashboard")
	flag.IntVar(&intervalMs, "interval", 2000, "Monitoring interval in milliseconds")
}

// monitorHost simulates pinging a host and updates the global status map.
// NOTE: In a production application, replace the simulation with an actual ICMP library.
func monitorHost(host string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	mu.Lock()
	hostStatuses[host] = HostStatus{
		Host:       host,
		Status:     "INIT",
		LatencyMs:  0,
		PacketLoss: 0,
		// LastCheck defaults to zero time (0001-01-01T00:00:00Z)
	}
	mu.Unlock()

	log.Printf("Starting monitoring for host: %s at %v intervals", host, interval)

	// Define a custom HTTP client with a timeout for the check
	client := http.Client{
		// Set a connection timeout to prevent checks from hanging indefinitely
		Timeout: 5 * time.Second,
	}

	for range ticker.C {
		var status string
		var latency float64 = 0.0
		var packetLoss float64 = 0.0 // Always 0% for a single HTTP check

		// Prepend scheme if missing for http.Client to work
		url := host
		if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
			url = "http://" + host // Default to HTTP for simplicity
		}

		startTime := time.Now()

		// Use a HEAD request, which is lighter than GET as it only requests headers
		req, err := http.NewRequest("HEAD", url, nil)
		if err != nil {
			log.Printf("Error creating request for %s: %v", host, err)
			status = "DOWN"
		} else {
			resp, err := client.Do(req)

			if err != nil {
				// Connection refused, timeout, or DNS error
				status = "DOWN"
				log.Printf("Host %s DOWN (Error: %v)", host, err)
			} else {
				defer resp.Body.Close()

				// Calculate actual latency
				latency = float64(time.Since(startTime).Microseconds()) / 1000.0 // Convert to milliseconds

				// A 2xx status code is generally considered UP
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					status = "UP"
				} else {
					status = "DOWN" // Treat non-2xx as a service failure
					log.Printf("Host %s DOWN (Status: %d)", host, resp.StatusCode)
				}
			}
		}

		mu.Lock()
		currentStatus := hostStatuses[host]
		currentStatus.Status = status
		// Use float64 for type conversion
		currentStatus.LatencyMs = float64(int(latency*100)) / 100.0   // Round to 2 decimals
		currentStatus.PacketLoss = float64(int(packetLoss*10)) / 10.0 // Round to 1 decimal
		currentStatus.LastCheck = time.Now()
		currentStatus.CheckCount++
		hostStatuses[host] = currentStatus
		mu.Unlock()
	}
}

// sseHandler handles the Server-Sent Events stream, pushing updates to the client.
func sseHandler(w http.ResponseWriter, r *http.Request) {
	// Set headers for Server-Sent Events
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Get a channel to detect when the client closes the connection
	ctx := r.Context()

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	// Initial data dump
	mu.RLock()
	statuses := hostStatuses
	mu.RUnlock()

	// Handle case where statuses map might be empty on rapid disconnect/reconnect
	if len(statuses) > 0 {
		data, _ := json.Marshal(statuses)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	// Loop to send updates every 500ms
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mu.RLock()
			// Only send data if there are hosts being monitored
			if len(hostStatuses) > 0 {
				statuses := hostStatuses
				mu.RUnlock()

				// Marshal and send the full set of statuses
				data, err := json.Marshal(statuses)
				if err != nil {
					log.Printf("Error marshalling JSON: %v", err)
					continue
				}

				// SSE format: data: {json_payload}\n\n
				_, err = fmt.Fprintf(w, "data: %s\n\n", data)
				if err != nil {
					// Client closed connection (likely)
					log.Printf("Client disconnected from SSE stream.")
					return
				}
				flusher.Flush()
			} else {
				mu.RUnlock()
			}

		case <-ctx.Done():
			// Client connection closed
			return
		}
	}
}

// indexHandler serves the main HTML dashboard template.
func indexHandler(w http.ResponseWriter, r *http.Request) {
	t, err := template.New("dashboard").Parse(htmlTemplate)
	if err != nil {
		http.Error(w, "Could not parse template", http.StatusInternalServerError)
		return
	}
	t.Execute(w, nil)
}

func main() {
	// Parse the flags here, after defining them in init()
	flag.Parse()

	rand.Seed(time.Now().UnixNano()) // Seed random for simulation

	log.Println("Starting Service Monitoring Service...")

	// 1. Start Service Monitoring Goroutines
	hosts := strings.Split(hostsStr, ",")
	interval := time.Duration(intervalMs) * time.Millisecond

	if len(hosts) == 0 || (len(hosts) == 1 && hosts[0] == "") {
		log.Fatal("No hosts specified. Please use the -hosts flag.")
	}

	filteredHosts := make([]string, 0)
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host != "" {
			filteredHosts = append(filteredHosts, host)
			go monitorHost(host, interval)
		}
	}

	// 2. Setup HTTP routes
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/events", sseHandler)

	// 3. Start Web Server
	addr := ":" + strconv.Itoa(port)
	log.Printf("Web Dashboard available at http://localhost%s", addr)
	// Log the confirmed settings
	log.Printf("Monitoring %d hosts (Interval: %dms, Port: %d)", len(filteredHosts), intervalMs, port)

	err := http.ListenAndServe(addr, nil)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// The HTML/CSS/JavaScript template for the dashboard
const htmlTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Host Monitor Dashboard in GoLang for Linux</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;600;700&display=swap" rel="stylesheet">
    <style>
        body { font-family: 'Inter', sans-serif; background-color: #f7fafc; }
        .card { transition: all 0.3s ease; }
        .status-up { background-color: #d1fae5; color: #065f46; border-left: 4px solid #10b981; }
        .status-down { background-color: #fee2e2; color: #991b1b; border-left: 4px solid #ef4444; animation: pulse-down 1.5s infinite; }
        .status-init { background-color: #eff6ff; color: #1e40af; border-left: 4px solid #3b82f6; }
        @keyframes pulse-down {
            0%, 100% { box-shadow: 0 0 10px rgba(239, 68, 68, 0.4); }
            50% { box-shadow: 0 0 20px rgba(239, 68, 68, 0.8); }
        }
    </style>
</head>
<body class="p-4 md:p-8">

    <header class="mb-8">
        <h1 class="text-4xl font-extrabold text-gray-900 tracking-tight">
            Host Monitor Dashboard in GoLang for Linux
        </h1>
        <p class="text-lg text-gray-500 mt-2">
            Real-time status via Server-Sent Events (SSE).
        </p>
    </header>

    <div id="loading" class="text-center py-12 text-gray-500 text-lg">
        <svg class="animate-spin h-8 w-8 text-blue-500 mx-auto mb-3" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
        </svg>
        Awaiting initial data stream...
    </div>

    <div id="dashboard" class="hidden">
        <div class="grid grid-cols-1 lg:grid-cols-3 gap-6 mb-8">
            <!-- Summary Cards will go here -->
            <div id="totalHosts" class="card bg-white p-6 rounded-xl shadow-lg border-l-4 border-gray-200">
                <p class="text-sm font-medium text-gray-500">Total Hosts</p>
                <p class="text-3xl font-bold text-gray-900 mt-1">0</p>
            </div>
            <div id="upHosts" class="card bg-white p-6 rounded-xl shadow-lg status-up">
                <p class="text-sm font-medium text-gray-600">Hosts UP</p>
                <p class="text-3xl font-bold text-green-700 mt-1">0</p>
            </div>
            <div id="downHosts" class="card bg-white p-6 rounded-xl shadow-lg border-l-4 border-gray-200">
                <p class="text-sm font-medium text-gray-600">Hosts DOWN</p>
                <p class="text-3xl font-bold text-red-700 mt-1">0</p>
            </div>
        </div>

        <h2 class="text-2xl font-semibold text-gray-800 mb-4">Host Details</h2>
        <div class="shadow-xl rounded-xl overflow-hidden bg-white">
            <table class="min-w-full divide-y divide-gray-200">
                <thead class="bg-gray-50">
                    <tr>
                        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Host</th>
                        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Status</th>
                        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Latency (ms)</th>
                        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Packet Loss (%)</th>
                        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Last Check</th>
                    </tr>
                </thead>
                <tbody id="hostTableBody" class="bg-white divide-y divide-gray-200">
                    <!-- Rows will be injected here by JavaScript -->
                </tbody>
            </table>
        </div>
    </div>

    <script>
        document.addEventListener('DOMContentLoaded', () => {
            const loadingEl = document.getElementById('loading');
            const dashboardEl = document.getElementById('dashboard');
            const tableBody = document.getElementById('hostTableBody');

            // Summary Elements
            const totalHostsEl = document.querySelector('#totalHosts p:last-child');
            const upHostsEl = document.querySelector('#upHosts p:last-child');
            const downHostsEl = document.querySelector('#downHosts p:last-child');
            const downHostCard = document.getElementById('downHosts');

            // Open the SSE connection to the server
            const eventSource = new EventSource('/events');

            eventSource.onmessage = (event) => {
                try {
                    // Data is received as a single JSON object (map of hosts)
                    const data = JSON.parse(event.data);
                    
                    // Show the dashboard once data starts flowing
                    loadingEl.classList.add('hidden');
                    dashboardEl.classList.remove('hidden');

                    renderDashboard(data);
                } catch (e) {
                    console.error("Error parsing SSE JSON data:", e);
                    // Log the raw data to check format issues
                    console.log("Raw data:", event.data); 
                }
            };

            eventSource.onerror = (err) => {
                console.error("EventSource failed:", err);
                eventSource.close();
            };

            function renderDashboard(statuses) {
                let upCount = 0;
                let downCount = 0;
                
                let html = '';
                
                // Get sorted host keys for stable table order
                const hosts = Object.keys(statuses).sort();

                hosts.forEach(hostKey => {
                    const status = statuses[hostKey];
                    
                    // The 'status' field is correct (lowercase)
                    const statusClass = 'status-' + status.status.toLowerCase();
                    
                    if (status.status === 'UP') upCount++;
                    if (status.status === 'DOWN') downCount++;

                    let lastCheckTime = 'N/A';
                    
                    // FIX: Use status.lastCheck (camelCase) to match JSON output
                    // FIX: Check for Go's zero time string ("0001-01-01T00:00:00Z") which crashes JS Date parsing
                    if (status.lastCheck && !status.lastCheck.startsWith('0001-01-01')) {
                        // Safely convert and format the date
                        lastCheckTime = new Date(status.lastCheck).toLocaleTimeString();
                    }

                    html += '<tr class="hover:bg-gray-50 ' + statusClass + '">' +
                        // FIX: Use status.host (lowercase)
                        '<td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">' + status.host + '</td>' +
                        '<td class="px-6 py-4 whitespace-nowrap text-sm font-bold">' + status.status + '</td>' +
                        
                        // FIX: Use status.latencyMs (camelCase) which caused the 'toFixed' error
                        '<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-700">' +
                            (status.latencyMs > 0 ? status.latencyMs.toFixed(2) + 'ms' : '---') +
                        '</td>' +
                        
                        // FIX: Use status.packetLoss (camelCase) which also caused the 'toFixed' error
                        '<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-700">' +
                            status.packetLoss.toFixed(1) + '%' +
                        '</td>' +
                        '<td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">' +
                            lastCheckTime +
                        '</td>' +
                    '</tr>';
                });

                // Update Summary Cards
                totalHostsEl.textContent = hosts.length;
                upHostsEl.textContent = upCount;
                downHostsEl.textContent = downCount;
                
                // Update Down Card visual status
                if (downCount > 0) {
                    downHostCard.classList.add('status-down');
                } else {
                    downHostCard.classList.remove('status-down');
                }

                // Update Table
                tableBody.innerHTML = html;
            }
        });
    </script>
</body>
</html>
`
