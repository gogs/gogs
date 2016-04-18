Name: gogs
Version: 0.9
Release: 20

Summary: A painless self-hosted Git service
License: MIT
Packager: T.J. Yang <tjyang2001@gmail.com>
Group: Applications/System
Url: https://github.com/gogits/gogs
Source0: gogs-0.9.20.tar.gz
BuildArch: x86_64
BuildRoot: %{_tmppath}/%{name}-%{version}-root
BuildRequires: golang
# Allow newer compiler to suppress warnings
%if 0%{?el6}
Requires:  sqlite,mysql-server
%else
Requires:  sqlite,mariadb-server,mariadb
%endif



%description
Gogs is a painless self-hosted Git Service written in Go. It aims to make the easiest, fastest and most painless way to set up a self-hosted Git service. With Go, this can be done in independent binary distribution across ALL platforms that Go supports, including Linux, Mac OS X, and Windows.

%prep

%setup -q -n %{name}-0.9.20

%install
rm -rf %{buildroot}
mkdir -p %{buildroot}
mkdir -p %{buildroot}/usr/local/gogs
mkdir -p %{buildroot}/var/log/gogs
touch  %{buildroot}/var/log/gogs/gogs.log

mkdir -p $RPM_BUILD_ROOT%{_localstatedir}/gogs/log
rsync -azp  .  --exclude=gogs/rpmbuild  --exclude=#.* %{buildroot}/usr/local/gogs

# systemd gogs.service file
mkdir -p %{buildroot}/etc/systemd/system
sudo rsync -azpv  %{buildroot}/usr/local/gogs/contrib/gogs.service  %{buildroot}/etc/systemd/system/gogs.service
#touch /var/log/gogs.log && chown gogs:gogs /var/log/gogs.log
%preun
#shutdown gogs if running
#if [ $1 = 0 ] ; then
#/sbin/install-info --delete %{_infodir}/%{name}.info %{_infodir}/dir || :
#fi

# https://www.redhat.com/archives/rpm-list/2008-September/msg00007.html
%files
%ghost %{_localstatedir}/log/gogs/%{name}.log
%attr(-,gogs,gogs) %{_localstatedir}/log/gogs/%{name}.log

%ghost /etc/systemd/system/gogs.service
%attr(-,root,root) /etc/systemd/system/gogs.service

%dir /usr/local/%{name}
%attr(-,gogs,gogs) /usr/local/%{name}

%post
# remove all the binary and configs
# hack before fix
rsync -azp  /usr/local/gogs/contrib/gogs.service  /etc/systemd/system/gogs.service
chmod 755    /etc/systemd/system/gogs.service
chown root:root /etc/systemd/system/gogs.service
# run a mysql -root to drop  all data in gogsdb  ?
#
#
%changelog
* Sun Apr 17 2016 T.J. Yang <tjyang2001@gmail.com> 1.0
- Initial rpm.spec file for gogs.
- TODOs:
- 1. Adding pre/post install script
- 2. Support CentOS 7 systemd and Centos 6 init scripts.
- 3. roll in the go build -x process into this rpm one.
- 4. Detect packageio.in's gogs rpm package.
- 5. remove all gogs db in mariadb also ?




