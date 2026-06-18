# Installing DaemonHound

DaemonHound ships as a single `dhd` binary plus optional package-manager artifacts from GitHub Releases.

## Linux

### Install Script

This is the quickest path on most Linux machines. It detects your OS and CPU architecture, downloads the latest GitHub Release archive, and installs `dhd` to `/usr/local/bin` by default.

```bash
curl -fsSL https://raw.githubusercontent.com/0xdps/daemon-hound/trunk/install.sh | sh
dhd version
```

Install to a user-writable directory instead:

```bash
mkdir -p ~/.local/bin
curl -fsSL https://raw.githubusercontent.com/0xdps/daemon-hound/trunk/install.sh | INSTALL_DIR=$HOME/.local/bin sh
dhd version
```

Make sure the chosen directory is in `PATH`.

### Debian / Ubuntu

Download the `.deb` asset from the latest release, then install it:

```bash
sudo dpkg -i daemon-hound_*_linux_amd64.deb
sudo apt-get install -f
dhd version
# or use the daemon-hound alias:
daemon-hound version
```

Use the `arm64` package on ARM64 systems.

### Fedora / RHEL / openSUSE

Download the `.rpm` asset from the latest release, then install it:

```bash
sudo rpm -i daemon-hound_*_linux_amd64.rpm
dhd version
# or use the daemon-hound alias:
daemon-hound version
```

Use the `arm64` packages on ARM64 systems.

### Alpine Linux

```bash
sudo apk add --allow-untrusted daemon-hound_*_linux_amd64.apk
dhd version
# or use the daemon-hound alias:
daemon-hound version
```

### Arch Linux

```bash
sudo pacman -U daemon-hound_*_linux_amd64.pkg.tar.zst
dhd version
# or use the daemon-hound alias:
daemon-hound version
```

### Direct Archive Download

For x86_64 Linux:

```bash
RELEASE_TAG=v1.1.0
VERSION=${RELEASE_TAG#v}
curl -LO "https://github.com/0xdps/daemon-hound/releases/download/${RELEASE_TAG}/daemon-hound_${VERSION}_Linux_x86_64.tar.gz"
tar -xzf "daemon-hound_${VERSION}_Linux_x86_64.tar.gz"
sudo install -m 0755 dhd /usr/local/bin/dhd
dhd version
```

For ARM64 Linux, use `daemon-hound_${VERSION}_Linux_arm64.tar.gz`.

## macOS

### Homebrew

```bash
brew tap 0xdps/packages
brew install daemon-hound
dhd version
```

### Install Script

```bash
curl -fsSL https://raw.githubusercontent.com/0xdps/daemon-hound/trunk/install.sh | sh
dh version
```

## Windows

### Scoop

```powershell
scoop bucket add daemon-hound https://github.com/0xdps/scoop-bucket
scoop install daemon-hound
dhd version
```

### Portable ZIP

Download the Windows `.zip` asset from the latest release, extract it, and place `dhd.exe` somewhere in `PATH`.

## Initialize DaemonHound

First machine:

```bash
dhd init --remote git@github.com:you/my-vault.git
dhd daemon status
```

Second machine:

```bash
# On the first machine
dhd export-identity

# On the second machine
dhd init --remote git@github.com:you/my-vault.git --age-key AGE-SECRET-KEY-...
dhd discover ~/projects
```

`dhd init` installs a user-level background daemon where supported:

- Linux: systemd user service at `~/.config/systemd/user/daemon-hound.service`
- macOS: launchd user agent
- Windows: Task Scheduler task

## Updating

Install the newer release with the same method you used originally, then restart the daemon so the service runs the new binary:

```bash
dhd daemon restart
dhd daemon status
dhd version
```

On Linux, verify which binary the user service runs:

```bash
systemctl --user cat daemon-hound.service
systemctl --user status daemon-hound.service --no-pager
```

If `ExecStart` points at a development checkout such as `.../daemon-hound/bin/dh`, reinstall or reinitialize from the installed `dh` path so the daemon uses the release binary.

## Uninstalling

Remove local DaemonHound state and the background daemon from the current machine:

```bash
dh cleanup
```

Then remove the binary or package using the same method used for installation:

```bash
sudo rm -f /usr/local/bin/dh
# or: brew uninstall daemon-hound
# or: scoop uninstall daemon-hound
# or your Linux package manager's remove command
```

`dh cleanup` does not delete your remote vault repository.