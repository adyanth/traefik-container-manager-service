package main

import (
	// "context"
	// "fmt"
	// "log"
	// "net/http"
	// "net/url"
	// "strconv"
	// "time"

	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

// Status is the service status
type Status string

const (
	// UP represents a service that is running (with at least a container running)
	UP Status = "running"
	// DOWN represents a service that is not running (with 0 container running)
	DOWN Status = "down"
	// STARTING represents a service that is starting (with at least a container starting)
	STARTING Status = "starting"
	// UNKNOWN represents a service for which the docker status is not know
	UNKNOWN Status = "unknown"
)

// Service holds all information related to a service
type Service struct {
	name      string
	timeout   uint64
	time      chan uint64
	isHandled bool
}

var services = map[string]*Service{}

func main() {
	fmt.Println("Server listening on port 10000.")
	http.HandleFunc("/", handleRequests())
	log.Fatal(http.ListenAndServe(":10000", nil))
}

func handleRequests() func(w http.ResponseWriter, r *http.Request) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		log.Fatal(fmt.Errorf("%+v", "Could not connect to docker API"))
	}
	return func(w http.ResponseWriter, r *http.Request) {
		serviceName, serviceTimeout, err := parseParams(r)
		fmt.Println(serviceName, serviceTimeout)
		if err != nil || serviceName == "" || serviceTimeout == 0 {
			fmt.Fprintf(w, "error: %+v, service name = `%s`, timeout = `%d`", err, serviceName, serviceTimeout)
			return
		}
		service := GetOrCreateService(serviceName, serviceTimeout)
		status, err := service.HandleServiceState(cli)
		if err != nil {
			fmt.Printf("Error: %+v\n ", err)
			fmt.Fprintf(w, "%+v", err)
		}
		fmt.Fprintf(w, "%+s", status)
	}
}

func getParam(queryParams url.Values, paramName string) (string, error) {
	if queryParams[paramName] == nil {
		return "", fmt.Errorf("%s is required", paramName)
	}
	return queryParams[paramName][0], nil
}

func parseParams(r *http.Request) (string, uint64, error) {
	queryParams := r.URL.Query()

	serviceName, err := getParam(queryParams, "name")
	if err != nil {
		return "", 0, nil
	}

	timeoutString, err := getParam(queryParams, "timeout")
	if err != nil {
		return "", 0, nil
	}
	serviceTimeout, err := strconv.Atoi(timeoutString)
	if err != nil {
		return "", 0, fmt.Errorf("timeout should be an integer")
	}
	return serviceName, uint64(serviceTimeout), nil
}

func GetOrCreateService(name string, timeout uint64) *Service {
	if services[name] != nil {
		return services[name]
	}
	service := &Service{name, timeout, make(chan uint64, 1), false}

	services[name] = service
	return service
}

// HandleServiceState up the service if down or set timeout for downing the service
func (service *Service) HandleServiceState(cli *client.Client) (string, error) {
	status, err := service.getStatus(cli)
	// return "", fmt.Errorf("state: %s, %+v", status, err)
	if err != nil {
		return "", err
	}
	if status == UP {
		fmt.Printf("- Service %v is up\n", service.name)
		fmt.Println(service)
		if !service.isHandled {
			go service.stopAfterTimeout(cli)
		}
		select {
		case service.time <- service.timeout:
			fmt.Println("Sent delay")
		default:
		}
		return "started", nil
	} else if status == STARTING {
		fmt.Printf("- Service %v is starting\n", service.name)
		if err != nil {
			return "", err
		}
		go service.stopAfterTimeout(cli)
		return "starting", nil
	} else if status == DOWN {
		fmt.Printf("- Service %v is down\n", service.name)
		service.start(cli)
		return "starting", nil
	} else {
		fmt.Printf("- Service %v status is unknown\n", service.name)
		if err != nil {
			return "", err
		}
		return service.HandleServiceState(cli)
	}
}

func (service *Service) getStatus(client *client.Client) (Status, error) {
	ctx := context.Background()
	var status Status = UNKNOWN
	containers, err := service.getDockerContainers(ctx, client)
	if err != nil {
		return status, err
	}
	for _, container := range containers {
		switch container.State {
		case "running":
		default:
			status = DOWN
		}
	}
	if status != DOWN {
		status = UP
	}

	return status, nil
}

func (service *Service) start(client *client.Client) {
	fmt.Printf("Starting service %s\n", service.name)
	service.isHandled = true
	service.startContainers(client)
	go service.stopAfterTimeout(client)
	service.time <- service.timeout
}

func (service *Service) stopAfterTimeout(client *client.Client) {
	service.isHandled = true
	fmt.Println("In stopAfterTimeout")

	if timeout, ok := <-service.time; ok {
		fmt.Println("Sleeping", timeout)
		time.Sleep(time.Duration(timeout) * time.Second)
	} else {
		fmt.Println("That should not happen, but we never know ;)")
	}

	for {
		select {
		case timeout, ok := <-service.time:
			if ok {
				fmt.Println("Sleeping", timeout)
				time.Sleep(time.Duration(timeout) * time.Second)
			} else {
				fmt.Println("That should not happen, but we never know ;)")
			}
		default:
			fmt.Printf("Stopping service %s\n", service.name)
			service.stopContainers(client)
			return
		}
	}
}

func (service *Service) startContainers(client *client.Client) error {
	ctx := context.Background()
	containers, err := service.getDockerContainers(ctx, client)
	if err != nil {
		return err
	}
	for _, container := range containers {
		if container.State != "running" {
			if err := client.ContainerStart(ctx, container.ID, types.ContainerStartOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (service *Service) stopContainers(client *client.Client) error {
	ctx := context.Background()
	containers, err := service.getDockerContainers(ctx, client)
	if err != nil {
		return err
	}
	for _, container := range containers {
		fmt.Println(container.Image, container.State)
		if container.State == "running" {
			if err := client.ContainerStop(ctx, container.ID, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func (service *Service) getDockerContainers(ctx context.Context, client *client.Client) ([]types.Container, error) {
	opts := types.ContainerListOptions{All: true}
	opts.Filters = filters.NewArgs()
	opts.Filters.Add("label", fmt.Sprintf("%s=%s", "traefik-manager-name", service.name))
	services, err := client.ContainerList(context.Background(), opts)

	if err != nil {
		return nil, err
	}
	return services, nil
}
