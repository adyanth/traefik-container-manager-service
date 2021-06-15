# traefik-container-manager-service

This works in combination with the `traefik-container-manager` middleware.

The API takes a name, timeout and optional host and path to use when the traefik config is made with labels rather than dynamic config.

Host and path matching is just a prefix match on host or path portion. If both are provided, both will be checked one after the other. If any one has a match, it will be considered.

To add the labels for this to recognize, use `traefik-container-manager.name` (mandatory), `traefik-container-manager.host` and `traefik-container-manager.path`. The name of `generic-container-manager` is reserved for when the labels are used for configuration, and a fallback of `path` or `host`, which is matched to the HTTP request trying to wake the container (basically traefik's Host rule or the PathPrefix rule on the router), is needed to wake them up. Use this manager container itself to create that middleware.

The name label can be added to all the containers that need to be stopped along with the service where the middleware is defined on.

Have these labels below as reference:

```yaml
labels: 
      - traefik.enable=true
      - traefik.http.routers.manager.entrypoints=entryhttp
      - traefik.http.routers.manager.rule=HostRegexp(`{host:.+}`)
      - traefik.http.routers.manager.priority=1
      - traefik.http.middlewares.manager.errors.status=404
      - traefik.http.middlewares.manager.errors.service=manager
      - traefik.http.middlewares.manager.errors.query=/
      - traefik.http.routers.manager.middlewares=manager-starter
      - traefik.http.services.manager.loadbalancer.server.port=80
      - traefik.http.middlewares.manager-starter.plugin.traefik-container-manager.name=generic-container-manager
```
