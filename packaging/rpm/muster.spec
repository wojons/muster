Name:           muster
Version:        1.0.0
Release:        1%{?dist}
Summary:        Modern API Client and Development Tool
License:        MIT
URL:            https://muster.sh
Source0:        %{name}-%{version}.tar.gz

BuildArch:      x86_64
Requires:       ca-certificates
Recommends:     gnome-keyring

%description
Muster is a powerful CLI tool for API development and testing.
Features OpenAPI integration, authentication management, and
extensible workflows.

%prep
%setup -q

%build
# Binary is pre-built
cp %{_sourcedir}/muster-linux-amd64 muster

%install
install -D -m 755 muster %{buildroot}%{_bindir}/muster
install -D -m 644 systemd/muster-daemon.service %{buildroot}%{_unitdir}/muster-daemon.service
install -D -m 644 systemd/muster-daemon.socket %{buildroot}%{_unitdir}/muster-daemon.socket
install -D -m 644 desktop/muster.desktop %{buildroot}%{_datadir}/applications/muster.desktop

# Create directories
mkdir -p %{buildroot}%{_sysconfdir}/muster
mkdir -p %{buildroot}%{_sharedstatedir}/muster
mkdir -p %{buildroot}%{_localstatedir}/log/muster
mkdir -p %{buildroot}%{_rundir}/muster

%pre
getent group muster >/dev/null || groupadd -r muster
getent passwd muster >/dev/null || useradd -r -g muster -d %{_sharedstatedir}/muster -s /sbin/nologin muster

%post
%systemd_post muster-daemon.socket
%systemd_post muster-daemon.service

%preun
%systemd_preun muster-daemon.service
%systemd_preun muster-daemon.socket

%postun
%systemd_postun_with_restart muster-daemon.socket

%files
%license LICENSE
%doc README.md
%{_bindir}/muster
%{_unitdir}/muster-daemon.service
%{_unitdir}/muster-daemon.socket
%{_datadir}/applications/muster.desktop
%dir %attr(755, muster, muster) %{_sysconfdir}/muster
%dir %attr(755, muster, muster) %{_sharedstatedir}/muster
%dir %attr(755, muster, muster) %{_localstatedir}/log/muster
%dir %attr(755, muster, muster) %{_rundir}/muster

%changelog
* Mon Mar 10 2026 Muster Team <team@muster.sh> - 1.0.0-1
- Initial release
