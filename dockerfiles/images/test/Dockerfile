FROM   stackbrew/ubuntu:saucy

RUN    apt-get install -y --force-yes software-properties-common  
RUN    add-apt-repository -y "deb http://archive.ubuntu.com/ubuntu $(lsb_release -sc) universe"  
RUN    apt-get --yes --force-yes update  
RUN    apt-get --yes --force-yes upgrade  

