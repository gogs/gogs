RPM=rpm

ALL=$(RPM)

WDIR=$(HOME)
##############################################################################
# standard targets (all, clean, distclean, devclean, install)

all: $(ALL)

clean:
	find .. -type f -name "*~" -exec rm  -f  {} \;
	(rm -rf $(WDIR)/rpmbuild/RPMS/x86_64/gogs-0.9-20.x86_64.rpm \
           $(WDIR)/rpmbuild/SRPMS/gogs-0.9-20.src.rpm \
          ../gogs-0.9.20.tar.gz)
 

# rpm making automation for CentOS/RHEL.
ARCH ?= $(shell arch)
ifeq ($(ARCH),x86_64)
RPM_ARCH := x86_64
else
    ifeq ($(ARCH),i686)
RPM_ARCH := i386
    else
$(error Unknown arch "$(ARCH)".)
    endif
endif

# RPM_ARCH := noarch
rpm:
	# @create gogs tar ball.
	(cd ..;rm -f gogs-0.9.20)
	(cd ..;ln -s . gogs-0.9.20)
	(cd ..;tar zhcf gogs-0.9.20.tar.gz --exclude gogs-0.9.20/gogs-0.9.20.tar.gz --exclude gogs-0.9.20/gogs-0.9.20 --exclude RCS --exclude CVS --exclude build-* --exclude *~ --exclude .git* gogs-0.9.20/)
	(cd ..;rm -f gogs-0.9.20)
	# build the rpm using rpmbuild from ./rmbuild as topdir
	rm -rf ${WDIR}/rpmbuild && mkdir -p ${WDIR}/rpmbuild/SOURCES
	cp  ../gogs-0.9.20.tar.gz ${WDIR}/rpmbuild/SOURCES/gogs-0.9.20.tar.gz
	rpmbuild -ba --define "_topdir ${WDIR}/rpmbuild"   ./gogs.spec
gitcommit:
	git commit -a -m "lazy commit via make gitpush"

gitpush:
	git push origin develop

installrpm: 
	sudo $(RPM) -Uvh $(home)/rpmbuild/RPMS/x86_64/gogs-0.9-20.x86_64.rpm
removerpm:
	sudo $(RPM) -e gogs
