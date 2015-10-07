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
	client        *docker.Client
	wg            sync.WaitGroup
	hostContainer map[string]*ContainerPort
)

const (
	TICKER_TIME            = 60
	STOP_CONTAINER_TIMEOUT = 5
	START_CONTAINER_WAIT   = 10
	READ_WRITE_TIMEOUT     = 20
)

func main() {
	endpoint := "unix:///var/run/docker.sock"
	client, _ = docker.NewClient(endpoint)

	hostContainer = map[string]*ContainerPort{
		"foo.local.info":  &ContainerPort{Name: "foo", Port: "3000", LastAccess: time.Now()},
		"bar.local.info":  &ContainerPort{Name: "bar", Port: "3001", LastAccess: time.Now()},
		"test.local.info": &ContainerPort{Name: "test", Port: "3002", LastAccess: time.Now()},
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

	for _, c := range hostContainer {
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

type ContainerPort struct {
	Name       string
	Port       string
	LastAccess time.Time
}

func proxy(w http.ResponseWriter, r *http.Request) {

	// change the request host to match the target
	u, _ := url.Parse("http://127.0.0.1:8080")

	currentContainer := hostContainer[r.Host]

	log.Println("check if container is running...", currentContainer)
	container, er := client.InspectContainer(currentContainer.Name)

	if er != nil {
		log.Println("Error: ", er)
	}

	currentContainer.LastAccess = time.Now()

	if !container.State.Running {
		log.Println("starting container: ", currentContainer.Name)

		var hostConfig docker.HostConfig

		hostConfig.PortBindings = map[docker.Port][]docker.PortBinding{
			docker.Port("80/tcp"): {{HostPort: currentContainer.Port}},
		}
		if err := client.StartContainer(currentContainer.Name, &hostConfig); err != nil {
			log.Println("Error: ", err)
		}
		time.Sleep(START_CONTAINER_WAIT * time.Second)
		fmt.Println("started container! :)")
	}

	proxy := http.StripPrefix("", httputil.NewSingleHostReverseProxy(u))
	// You can optionally capture/wrap the transport if that's necessary (for
	// instance, if the transport has been replaced by middleware). Example:
	// proxy.Transport = &myTransport{proxy.Transport}
	// proxy.Transport = &myTransport{}

	proxy.ServeHTTP(w, r)
}
