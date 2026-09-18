<img src="https://img.shields.io/badge/Go-1.25.13-00ADD8?logo=go&logoColor=white"> <img src="https://img.shields.io/badge/platform-Linux-333">

English | [日本語](README.ja.md)

![hijo Server Ops](hso-animation.gif)

## hijo Server Ops

Tired of setting up server on LINUX? Don't worry, it would be so easy with HSO!

A TUI console for Minecraft servers on Linux. It runs as a wrapper around your server, so the start script you already use stays as it is.
Works with Vanilla, Spigot, Paper, Forge, NeoForge, Fabric and other setups.

## Quick start

### User Install

Installs into `~/.local/bin`. No root privileges are needed, but you may need to add it to your PATH.

```bash
curl -fsSL https://raw.githubusercontent.com/hijoushoku7/hijo-server-ops/main/install.sh | sh

curl -fsSL https://raw.githubusercontent.com/hijoushoku7/hijo-server-ops/main/install.sh | sh -s -- --lang ja # japanese version
```

If `~/.local/bin` is not on your PATH, the installer prints the single line to add.

### If you want it available to every user

No PATH setup is needed, but root privileges are required. **ROOT REQUIRED! (BUT DONT PUT SUDO, INSTALLER ASKS YOU!)**

```bash
curl -fsSL https://raw.githubusercontent.com/hijoushoku7/hijo-server-ops/main/install.sh | sh -s -- --system
```
Run `hso` or `hso help` for the list of commands (see [Command reference](dev-docs/commands.md), written in Japanese).

### How to set up your server

Run `hso setup` to open the setup wizard. It handles both cases:

- **You already have a server**: run it in that directory and pick the start script you already use from the list. The server comes up as it is.
- **You have nothing yet**: run it in the directory you want the server in and choose *install a new server*. Pick the Minecraft version and the loader (Vanilla, Paper, Fabric, Forge, NeoForge), agree to the EULA, and hso downloads and sets it up.

Either way the server is registered, so from the second time on `hso start` lets you choose one of the registered servers. Do not run hso itself with `sudo`.

```bash
hso setup     # first time: set up the server and start it
hso start     # after that: pick a registered server
```

## Features

- Interactive and **COOL** TUI console
- Easy Initial settings for multiple loader (Fabric,Forge,Neoforge,Vanilla,Paper)
- Heap and RSS (memory actually used) Graphs
- Easy commands to specific player
- Chat separated from console
- Auto restart
- Pick the Java version to run with
- editing server.properties

No plugin or mod has to be installed on the server side.


### Easy Server Setup

![hijo Server Ops](hso-setup.gif)

### Customizable theme

![hijo Server Ops](hso-theme.gif)

### Commands

![hijo Server Ops](hso-players.gif)

### Easy restarting
![hijo Server Ops](hso-restart.gif)


## Manual install

Download the archive matching your environment from [Releases](https://github.com/hijoushoku7/hijo-server-ops/releases) and extract it.

```bash
tar xzf hso_v0.*.*_linux_amd64_en.tar.gz
cd hso_v0.*.*_linux_amd64_en
./hso setup
```

Pick `arm64` for arm64 machines, and the `_ja` archive if you want the Japanese interface.

## Switching and checking Java

```bash
hso java change [name]
hso java list
```

`change` picks the Java a registered server uses from the JVMs found under `/usr/lib/jvm` and stores it in that server's `hso.toml`. You can change it while the server is running; it takes effect on the next start. `list` shows the JVMs that were found and which registered servers use each one. Java installed through SDKMAN, asdf, `/opt` and the like is not detected, so set the absolute JAVA_HOME path in `[server] java` of `hso.toml` to use it.

## Update

```bash
hso update
```
It's not that hard! Fetches the binary for the same architecture and interface language as the one currently running, verifies it with SHA-256, then replaces itself. If it is already up to date, nothing happens.

If it lives in `/usr/local/bin`, `sudo` / `doas` asks for your password **for the replacement step only**. There is no need to type `sudo hso update` (that would run the download and the extraction as root as well). In `~/.local/bin` it goes through without any elevation.

## Uninstall

```bash
hso uninstall
```
Don't worry! It just uninstall hso, not the server directories!
It prints the paths it is about to delete and asks you to confirm before removing the binary that is currently running. Add `-y` / `--yes` to skip that confirmation.

To remove only the binary installed in `/usr/local/bin`, run `sudo hso uninstall`. The uninstall never invokes `sudo` / `doas` by itself.

To remove the server list and the pidfile as well, add `--purge` and run it **without sudo**.

```bash
hso uninstall --purge
```

If it is installed in `/usr/local/bin`, delete the configuration as your normal user first, then remove the leftover binary as root.

```bash
hso uninstall --purge
sudo hso uninstall
```

If the binary is broken and `uninstall` cannot run, you can remove everything by hand.
If you set `HSO_INSTALL_DIR`, read the first line as the `hso` inside that directory.

```bash
rm "$HOME/.local/bin/hso"                 # or sudo rm /usr/local/bin/hso
rm -rf "${XDG_CONFIG_HOME:-$HOME/.config}/hso"
if [ -n "${XDG_RUNTIME_DIR:-}" ]; then
  rm -rf "$XDG_RUNTIME_DIR/hso"
else
  rm -rf "/tmp/hso-$(id -u)"
fi
```

The pidfile also goes away on reboot.

## Documentation

Written in Japanese.

- [Command reference](dev-docs/commands.md)
- [Build instructions](dev-docs/build.md)
- [Specification and technical notes](dev-docs/spec.md)

## License

MIT. See [LICENSE](LICENSE).

Bundled dependencies keep their own licenses (all MIT or BSD-3-Clause);
full texts are in [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md).

## Author

hijoushoku https://github.com/hijoushoku7
A Student Engineer from Japan🗾
