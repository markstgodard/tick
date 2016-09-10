# ❤ tick ❤
Simple app that heartbeats itself with an [a8registry](https://github.com/amalgam8/registry) 
and tests container to container connectivity.

## Usage
Assuming you have an a8registry app deployed and know its overlay address (i.e. 10.255.67.67)
Push app, allow access to registry and start app
```
cf push tick --no-start
cf set-env tick REGISTRY_HOST "10.255.67.67:8080"
cf access-allow tick registry --protocol tcp --port 8080
cf start tick
```
