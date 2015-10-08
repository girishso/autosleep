package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/fsouza/go-dockerclient"
)

var (
	client            *docker.Client
	wg                sync.WaitGroup
	hostContainerInfo map[string]*ContainerInfo // maps hostname to ContainerInfo
	idContainerInfo   map[string]*ContainerInfo // maps ID to ContainerInfo
)

const (
	TICKER_TIME            = 30
	STOP_CONTAINER_TIMEOUT = 5
	START_CONTAINER_WAIT   = 5
	READ_WRITE_TIMEOUT     = 10
)

type ContainerInfo struct {
	ID            string
	Name          string
	HostPort      string
	ContainerPort string
	Running       bool
	LastAccess    time.Time
}

func main() {
	endpoint := "unix:///var/run/docker.sock"
	client, _ = docker.NewClient(endpoint)

	hostContainerInfo = map[string]*ContainerInfo{
		"foo.local.info":     &ContainerInfo{Name: "foo", HostPort: "3000", ContainerPort: "80/tcp", LastAccess: time.Now()},
		"bar.local.info":     &ContainerInfo{Name: "bar", HostPort: "3001", ContainerPort: "80/tcp", LastAccess: time.Now()},
		"test.local.info":    &ContainerInfo{Name: "test", HostPort: "3002", ContainerPort: "80/tcp", LastAccess: time.Now()},
		"example.local.info": &ContainerInfo{Name: "rails_sample", HostPort: "3003", ContainerPort: "8080/tcp", LastAccess: time.Now()},
	}

	setAllContainerStates()

	go watchDockerEvernts()

	go func() {
		t := time.NewTicker(time.Second * (TICKER_TIME / 3))
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
		ReadTimeout:  READ_WRITE_TIMEOUT * time.Second,
		WriteTimeout: READ_WRITE_TIMEOUT * time.Second,
	}
	log.Fatal(s.ListenAndServe())
}

func setAllContainerStates() {
	idContainerInfo = make(map[string]*ContainerInfo)

	for _, c := range hostContainerInfo {
		dockerContainer := getDockerContainer(c)
		c.Running = dockerContainer.State.Running
		c.ID = dockerContainer.ID
		idContainerInfo[c.ID] = c
	}

}

func watchDockerEvernts() {
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

func stopInactiveContainers() {
	for _, c := range hostContainerInfo {
		d := time.Now().Sub(c.LastAccess)
		if d.Seconds() > TICKER_TIME {
			if container, er := client.InspectContainer(c.Name); er != nil {
				log.Println("Error: ", er)
			} else if container.State.Running {
				log.Println("stopping container: ", c.Name, d.Seconds())
				if err := client.StopContainer(container.ID, STOP_CONTAINER_TIMEOUT); err != nil {
					log.Println("Error stopping container: ", err)
				} else {
					log.Println("Stopped container.")
				}
			}
		}
	}
}

func proxy(w http.ResponseWriter, r *http.Request) {

	u, _ := url.Parse("http://127.0.0.1:8080")

	currentContainerInfo := hostContainerInfo[r.Host]

	// set LastAccess
	currentContainerInfo.LastAccess = time.Now()

	if !currentContainerInfo.Running {
		log.Println("starting container: ", currentContainerInfo.Name)
		startContainer(currentContainerInfo)
	}

	proxy := http.StripPrefix("", httputil.NewSingleHostReverseProxy(u))

	proxy.ServeHTTP(w, r)
}

func startContainer(containerInfo *ContainerInfo) {
	var hostConfig docker.HostConfig

	hostConfig.PortBindings = map[docker.Port][]docker.PortBinding{
		docker.Port(containerInfo.ContainerPort): {{HostPort: containerInfo.HostPort}},
	}
	if err := client.StartContainer(containerInfo.Name, &hostConfig); err != nil {
		log.Println("Error: ", err)
	}
	time.Sleep(START_CONTAINER_WAIT * time.Second)
	fmt.Println("started container! :)")
}

func getDockerContainer(containerInfo *ContainerInfo) *docker.Container {
	log.Println("checking container status...", containerInfo)
	container, er := client.InspectContainer(containerInfo.Name)

	if er != nil {
		log.Println("Error: ", er)
	}

	return container

}
