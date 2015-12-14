package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/girishso/autosleep/Godeps/_workspace/src/github.com/fsouza/go-dockerclient"
)

var (
	client            *docker.Client
	wg                sync.WaitGroup
	hostContainerInfo map[string]*ContainerInfo // maps hostname to ContainerInfo
	idContainerInfo   map[string]*ContainerInfo // maps ID to ContainerInfo
	AutoSleepIn       int
)

const (
	StopTimeout        = 5
	StartContainerWait = 5
	ReadWriteTimeout   = 10
)

type ContainerInfo struct {
	ID          string
	Name        string
	PortBinding map[docker.Port][]docker.PortBinding
	Running     bool
	LastAccess  time.Time
	StartedAt   time.Time
}

func main() {
	flag.IntVar(&AutoSleepIn, "autosleepin", 60*30, "auto sleep containers in this many seconds")
	flag.Parse()

	log.SetLevel(log.WarnLevel)

	endpoint := "unix:///var/run/docker.sock"
	client, _ = docker.NewClient(endpoint)

	getAllDockerContainers()

	go watchDockerEvents()

	go func() {
		t := time.NewTicker(time.Second * time.Duration(AutoSleepIn/3))
		for {
			// instead of creating new Tickers for each container use only one Ticker,
			// good enuf for our purposes
			// doesn't matter if the container lives for a bit longer
			stopInactiveContainers()
			<-t.C
		}
	}()

	http.HandleFunc("/", proxy)

	s := &http.Server{
		Addr:         ":80",
		Handler:      nil,
		ReadTimeout:  ReadWriteTimeout * time.Second,
		WriteTimeout: ReadWriteTimeout * time.Second,
	}
	log.Fatal(s.ListenAndServe())
}

func getAllDockerContainers() {
	imgs, _ := client.ListContainers(docker.ListContainersOptions{All: true})
	hostContainerInfo = make(map[string]*ContainerInfo)
	idContainerInfo = make(map[string]*ContainerInfo)

	for _, img := range imgs {
		container, _ := client.InspectContainer(img.ID)
		vHost := splitKeyValueSlice(container.Config.Env)["VIRTUAL_HOST"]

		if vHost != "" {
			containerInfo := &ContainerInfo{
				ID:          container.ID,
				Name:        container.Name,
				PortBinding: container.HostConfig.PortBindings,
				Running:     container.State.Running,
				LastAccess:  time.Now(),
				StartedAt:   container.State.StartedAt}

			if existingContainer, ok := hostContainerInfo[vHost]; ok {
				log.Warningf("container: %s has the same VIRTUAL_HOST=%s as container: %s", container.ID[:12], vHost, existingContainer.ID[:12])
				if existingContainer.StartedAt.UnixNano() < container.State.StartedAt.UnixNano() {
					log.Warningf("using the most recently used container: %s, with VIRTUAL_HOST=%s", containerInfo.ID[:12], vHost)
					// only consider newer containers
					hostContainerInfo[vHost] = containerInfo

					// remove existing container from idContainerInfo
					delete(idContainerInfo, existingContainer.ID)
					idContainerInfo[container.ID] = containerInfo
				} else {
					log.Warningf("using the most recently used container: %s, with VIRTUAL_HOST=%s", existingContainer.ID[:12], vHost)
				}
			} else {
				hostContainerInfo[vHost] = containerInfo
				idContainerInfo[container.ID] = containerInfo
			}
		}
	}
}

func watchDockerEvents() {
	eventChan := make(chan *docker.APIEvents, 100)
	defer close(eventChan)

	watching := false
	for {

		if client == nil {
			break
		}
		err := client.Ping()
		if err != nil {
			log.Printf("Unable to ping docker daemon: %s", err)
			if watching {
				client.RemoveEventListener(eventChan)
				watching = false
				client = nil
			}
			time.Sleep(10 * time.Second)
			break

		}

		if !watching {
			err = client.AddEventListener(eventChan)
			if err != nil && err != docker.ErrListenerAlreadyExists {
				log.Printf("Error registering docker event listener: %s", err)
				time.Sleep(10 * time.Second)
				continue
			}
			watching = true
			log.Println("Watching docker events")
		}

		select {

		case event := <-eventChan:
			if event == nil {
				if watching {
					client.RemoveEventListener(eventChan)
					watching = false
					client = nil
				}
				break
			}

			if _, ok := idContainerInfo[event.ID]; ok {
				// if event.Status == "start" || event.Status == "stop" || event.Status == "die" {
				log.Printf("Received event %s for container %s", event.Status, event.ID[:12])

				switch event.Status {
				case "start":
					idContainerInfo[event.ID].Running = true
				case "die":
					idContainerInfo[event.ID].Running = false
				case "stop":
					idContainerInfo[event.ID].Running = false
				}
			}
		}
	}
}

func stopInactiveContainers() {
	for _, c := range hostContainerInfo {
		d := time.Now().Sub(c.LastAccess)
		if d.Seconds() > float64(AutoSleepIn) {
			if container, er := client.InspectContainer(c.Name); er != nil {
				log.Errorln(er)
			} else if container.State.Running {
				log.Println("stopping container: ", c.ID[:12], c.Name, d.Seconds())
				if err := client.StopContainer(container.ID, StopTimeout); err != nil {
					log.Errorln("Error stopping container: ", c.ID[:12], c.Name, err)
				} else {
					log.Println("Stopped container.", c.ID[:12], c.Name)
				}
			}
		}
	}
}

func proxy(w http.ResponseWriter, r *http.Request) {

	u, _ := url.Parse("http://127.0.0.1:8080")

	currentContainerInfo := hostContainerInfo[r.Host]

	if currentContainerInfo != nil {
		// set LastAccess
		currentContainerInfo.LastAccess = time.Now()

		if !currentContainerInfo.Running {
			log.Println("starting container: ", currentContainerInfo.ID[:12], currentContainerInfo.Name)
			startContainer(currentContainerInfo)
		}
	}

	proxy := http.StripPrefix("", httputil.NewSingleHostReverseProxy(u))

	proxy.ServeHTTP(w, r)
}

func startContainer(containerInfo *ContainerInfo) {
	var hostConfig docker.HostConfig

	hostConfig.PortBindings = containerInfo.PortBinding

	if err := client.StartContainer(containerInfo.ID, &hostConfig); err != nil {
		log.Errorln(err)
	}
	time.Sleep(StartContainerWait * time.Second)
	fmt.Printf("started container! %s, %s :)\n", containerInfo.ID[:12], containerInfo.Name)
}

// splitKeyValueSlice takes a string slice where values are of the form
// KEY, KEY=, KEY=VALUE  or KEY=NESTED_KEY=VALUE2, and returns a map[string]string where items
// are split at their first `=`.
func splitKeyValueSlice(in []string) map[string]string {
	env := make(map[string]string)
	for _, entry := range in {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			parts = append(parts, "")
		}
		env[parts[0]] = parts[1]
	}
	return env

}
