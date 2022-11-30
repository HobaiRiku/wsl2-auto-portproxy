# wsl2-auto-portProxy
wsl2-auto-portProxy(wslpp) is a simple tool for proxying port of linux running in wsl2 (which now use a hyper-v nat network), it automatically scans the port in wls and setup a port proxy in windows host.    

**Note: only port listening at [::] or 0.0.0.0 works, and will only works to  your default  wsl distribution**

## Feature
- [x] TCP port support
- [x] custom port proxy config, support live edit
- [ ] web interface
- [ ] UDP port support


## Requirement
~~your wsl linux must install the `net-tools` by~~ 
```bash
# deprecated !!!, not needed anymore
sudo apt-get install net-tools
```
**Note: `net-tools` is not required anymore, use `iproute2` instead 
(which is preinstalled by default in many linux distribution)**, 
see [Why is net-tools deprecated](https://unix.stackexchange.com/questions/677763/why-is-net-tools-deprecated-can-i-still-use-it-without-security-issue)


## Build and install
you can download the bin file(wslpp.exe) in [release](https://github.com/HobaiRiku/wsl2-auto-portproxy/releases).
#### or build wslpp.exe from source
```bash
make build
```
the bin file will be store in dist/wslpp.exe    

#### or install with `go get`
```bash
go get https://github.com/HobaiRiku/wsl2-auto-portproxy
```
and use `wsl2-auto-portproxy.exe` to start proxy

## How it works
wslpp start an interval to get IP address of the nat interface and scan all ports listening at all network in the subsystem, then use golang's `net` to start proxy direct to ports.

## Configuration
Support custom configuration by a json file, which must be placed in `%HOMEPATH%/.wslpp/config.json`, the `.wslpp` dir will be created automatically by wslpp when it runs, but the json file should be created by yourself.    
Example:
```json
{
  "onlyPredefined": true,
  "predefined": {
    "tcp": [
      "666:22"
    ]
  },
  "ignore": {
    "tcp": [
      445
    ]
  }
}
```
* onlyPredefined: If `true`, will only start port defined in `predefined` field.
* predefined: Define the custom port to proxy, "666:22" means `windows(666)->linux(22)`, if undefined, port in windows will follow the same of linux. Must be a string array in the sub field name `tcp`.
* ignore: If defined, will ignore the port in linux. Must be a number array in the sub field name `tcp`. 

**Note: If port is already use by another program in windows, the port will be omitted**

## About `wslhost.exe`
Now Microsoft will forward ports in linux by `wslhost.exe` when `.wslconfig` includes `localhostForwarding=true` (which is ture by default), see [wsl-config](https://learn.microsoft.com/en-us/windows/wsl/wsl-config). But, all those ports will only listen at local network on windows host, which means you can't access them from other devices in the same network. 
For now, `wslpp` will still open a same port listening at all interfaces, but if you don't need network access at this, you probably don't need `wslpp` at all, `wslhost.exe` is enough.
## Another solution for wsl2 port forwarding - `WSLHostPatcher` 
Fond a way to inject `wslhost.exe` to forward ports to all interfaces, 
[WSLHostPatcher](https://github.com/CzBiX/WSLHostPatcher).    
By `WSLHostPatcher` you can forward ports to all interfaces by `wslhost.exe` more gracefully and efficiently.

## Security issue
It is unsafe to open ports in windows host to the internet (maybe the main reason why wslhost.exe don't do this), so when start port at all interfaces, be sure you know what you are doing.

## License
MIT

