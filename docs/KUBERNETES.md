# Running godrain against Kubernetes

Services in Kubernetes run inside pods, so their pprof port is not reachable from your laptop. This guide shows how to reach it safely with `kubectl port-forward` and run `godrain watch` through that tunnel.

## 1. Open a tunnel to the pod

Your service must expose pprof on a port inside the pod (for example `6060`). Open a tunnel from your laptop to that port:

```bash
kubectl port-forward pod/my-app-pod 6060:6060
```

Replace `my-app-pod` with your pod name (find it with `kubectl get pods`). Keep this terminal open. The tunnel works only while this command is running.

You can also forward to a deployment instead of a single pod:

```bash
kubectl port-forward deploy/my-app 6060:6060
```

## 2. Run godrain through the tunnel

With the tunnel open, godrain can reach the service on your own machine. In a second terminal, run:

```bash
godrain watch --url http://127.0.0.1:6060 --duration 2m --interval 10s --warmup 15s
```

- `--url` is the pprof base URL (required). It points to the local end of the tunnel.
- `--duration` is how long godrain observes the service.
- `--interval` is how often it takes a sample.
- `--warmup` ignores early growth while the service starts up.

## 3. Security notes

- Never expose pprof publicly. `/debug/pprof` can reveal memory contents and internal details, so treat it as sensitive.
- Bind pprof to `localhost` or a private network inside the pod, not to `0.0.0.0` on a public service.
- `kubectl port-forward` is safe for this because it only listens on your own machine (`127.0.0.1`) by default.
- Close the tunnel (press `Ctrl + C` in that terminal) when you are done.
- See [SECURITY.md](../SECURITY.md) for more.

