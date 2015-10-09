# Autosleep inactive Docker Containers
I have many projects running on my linode slice, unfortunately they are not as popular as I hoped for! May be upto 10 users per day. They keep running consuming RAM and other resources with no work to do.

Couple of weeks back Docker got me interested, I wanted to get the free tier Heroku feature, sleeping off the inactive apps and activate them whenever someone accesses them. The result is `autosleep`!

# How does that work?
Autosleep creates a reverse proxy in front of `nginx`. It has all the bells and whistles to check if a Docker container is running or not, pass on the requests if it's running otherwise start it and then pass on the request. It stops the containers if they are inactive for about 30 mins.

Not bad for a week's work, Eh?? :)

# TODO
* Add config file, right now everything is hard coded :(
* Use docker-gen to auto generate config
* Upload binaries