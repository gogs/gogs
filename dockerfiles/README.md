### Install Gogs With Docker

Deploying gogs in [Docker](http://www.docker.io/) is just as easy as eating a pie, what you do is just run `run_gogs.sh`.
This will setup a MySQL container and a GoGS container.
The MySQL container will be linked to the GoGS container and everything will be setup for you.

After docker is done just visit http://localhost:3000 and setup the admin account and BAM you're done.

If you want to change the database user, password etc. do so in `run_gogs.sh`

| command        | description                                                      |
| -------------- | ---------------------------------------------------------------- |
| run_gogs.sh    | calls `docker run` to get, build and run MySQL and GoGS images   |
| stop_gogs.sh   | calls `docker stop` to stop MySQL and GoGS containers            |
| start_gogs.sh  | calls `docker start` to start MySQL and GoGS containers          |
| rm_gogs.sh     | calls `docker rm` to remove MySQL and GoGS containers            |
| rmi_gogs.sh    | calls `docker rmi` to remove GoGS image (leaves MySQL container) |

Have fun!
