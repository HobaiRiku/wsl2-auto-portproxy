# wsl2-auto-portProxy
wsl2-auto-portProxy(wslpp) is a simple tool for proxying port of linux running in wsl2 (which now use a hyper-v nat network), it automatically scans the port in wls and setup a port proxy in windows host.    
**Note: only port listening at [::] or 0.0.0.0 works.**

## Feature
- [x] TCP port support
- [x] custom port proxy config, support live edit
- [ ] web interface
- [ ] UDP port support

## Install


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

## Build
```bash
make build
```

## License
MIT

