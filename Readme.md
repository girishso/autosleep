# Autosleep inactive Docker Containers
I have many projects running on my linode slice, unfortunately they are not as popular as I hoped for! May be upto 10 users per day. They keep running consuming RAM and other resources with no work to do.

Couple of weeks back Docker got me interested, I wanted to get the free tier Heroku feature, sleeping off the inactive apps and activate them whenever someone accesses them. The result is `autosleep`!

# How does that work?
Autosleep creates a reverse proxy in front of `nginx`. It has all the bells and whistles to check if a Docker container is running or not, pass on the requests if it's running otherwise start it and then pass on the request. It stops the containers if they are inactive for about 30 mins.


# Installation

```
go get github.com/girishso/autosleep

(cd to github.com/girishso/autosleep)

godep go build autosleep.go

sudo ./autosleep
```

# Usage
Start containers with a VIRTUAL_HOST env variable (similar to docker-gen):

```
docker run -e VIRTUAL_HOST=foo.local.info -t ...
```

Autosleep also works with the stopped containers as well, but they must be created with `VIRTUAL_HOST` env. In case multiple
containers with the same `VIRTUAL_HOST` exist, `autosleep` uses the most recently run container.

Make sure config exists for `foo.local.info` in `nginx.conf`. Note nginx listens on port 8080.

```
upstream foo.local.info {
    server 127.0.0.1:3000;
}

server {
	listen 8080;
    gzip_types text/plain text/css application/json application/x-javascript
               text/xml application/xml application/xml+rss text/javascript;

    server_name foo.local.info;

    location / {
        proxy_pass http://foo.local.info;
        include /etc/nginx/proxy_params;
    }
}
```

Start autosleep

```
sudo ./autosleep
```

Have fun!


# TODO
* Create docker image
* Update instructions for using with `docker-gen`
* Demonize the process
* Upload binaries