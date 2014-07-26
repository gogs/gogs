FROM ubuntu

# Set the file maintainer (your name - the file's author)
MAINTAINER Borja Burgos <borja@tutum.co>

# Update the default application repository sources list
RUN apt-get update

# Install Memcached
RUN apt-get install -y memcached

# Port to expose (default: 11211)
EXPOSE 11211

# Default Memcached run command arguments
# Change to limit memory when creating container in Tutum 
CMD ["-m", "64"]

# Set the user to run Memcached daemon
USER daemon

# Set the entrypoint to memcached binary
ENTRYPOINT memcached

