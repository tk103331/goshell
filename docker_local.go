package main

import (
	"context"
	"os"
	"time"

	"github.com/docker/docker/client"
	"goshell/locales"
)

// dockerHost is the Docker daemon the app looks for: DOCKER_HOST when the
// user set one, else the client's default socket or pipe.
func dockerHost() string {
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		return host
	}
	return client.DefaultDockerHost
}

// isLocalDockerHost says whether a config's host is this machine's daemon:
// the default host, or empty, which the client also reads as the default.
func isLocalDockerHost(host string) bool {
	return host == "" || host == dockerHost()
}

// detectLocalDocker adds a Docker config for the local daemon the first
// time one answers. It runs in the background, and keeps a flag on disk so
// that a config the user deletes is not added again.
func (w *Window) detectLocalDocker() {
	if w.dockerDefaultAdded {
		return
	}
	// a config for the local daemon may already be saved, from an earlier
	// version or from the user: keep it, and do not add another
	for _, conf := range w.confs {
		if dock, ok := conf.(*DockerConfig); ok && isLocalDockerHost(dock.data.Host) {
			w.dockerDefaultAdded = true
			w.save()
			return
		}
	}

	go func() {
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			return
		}
		defer cli.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := cli.Ping(ctx); err != nil {
			// Docker may start later: ask again on the next launch
			return
		}

		w.onMain(func() {
			// the user may have added a config while the probe ran
			if w.dockerDefaultAdded {
				return
			}
			for _, conf := range w.confs {
				if dock, ok := conf.(*DockerConfig); ok && isLocalDockerHost(dock.data.Host) {
					w.dockerDefaultAdded = true
					w.save()
					return
				}
			}
			w.confs = append(w.confs, &DockerConfig{data: &DockerConfigData{
				Name: locales.LocalDockerName(),
				Type: "docker",
				Host: dockerHost(),
			}})
			w.dockerDefaultAdded = true
			w.save()
		})
	}()
}
