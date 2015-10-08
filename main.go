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
	hostContainerInfo map[string]*ContainerInfo
)

const (
	TICKER_TIME            = 30
	STOP_CONTAINER_TIMEOUT = 5
	START_CONTAINER_WAIT   = 5
	READ_WRITE_TIMEOUT     = 10
)

func main() {
	endpoint := "unix:///var/run/docker.sock"
	client, _ = docker.NewClient(endpoint)

	hostContainerInfo = map[string]*ContainerInfo{
		"foo.local.info":     &ContainerInfo{Name: "foo", Port: "3000", ContainerPort: "80/tcp", LastAccess: time.Now()},
		"bar.local.info":     &ContainerInfo{Name: "bar", Port: "3001", ContainerPort: "80/tcp", LastAccess: time.Now()},
		"test.local.info":    &ContainerInfo{Name: "test", Port: "3002", ContainerPort: "80/tcp", LastAccess: time.Now()},
		"example.local.info": &ContainerInfo{Name: "rails_sample", Port: "3003", ContainerPort: "8080/tcp", LastAccess: time.Now()},
	}

	go func() {
		t := time.NewTicker(time.Second * TICKER_TIME)
		for {
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

type ContainerInfo struct {
	Name          string
	Port          string
	ContainerPort string
	LastAccess    time.Time
}

func proxy(w http.ResponseWriter, r *http.Request) {

	// change the request host to match the target
	u, _ := url.Parse("http://127.0.0.1:8080")

	currentContainerInfo := hostContainerInfo[r.Host]

	currentContainerInfo.LastAccess = time.Now()

	if !isContainerRunning(currentContainerInfo) {
		log.Println("starting container: ", currentContainerInfo.Name)

		var hostConfig docker.HostConfig

		hostConfig.PortBindings = map[docker.Port][]docker.PortBinding{
			docker.Port(currentContainerInfo.ContainerPort): {{HostPort: currentContainerInfo.Port}},
		}
		if err := client.StartContainer(currentContainerInfo.Name, &hostConfig); err != nil {
			log.Println("Error: ", err)
		}
		time.Sleep(START_CONTAINER_WAIT * time.Second)
		fmt.Println("started container! :)")
	}

	proxy := http.StripPrefix("", httputil.NewSingleHostReverseProxy(u))

	proxy.ServeHTTP(w, r)
}

func isContainerRunning(containerInfo *ContainerInfo) bool {
	log.Println("check if container is running...", containerInfo)
	container, er := client.InspectContainer(containerInfo.Name)

	if er != nil {
		log.Println("Error: ", er)
	}

	return container.State.Running

}
