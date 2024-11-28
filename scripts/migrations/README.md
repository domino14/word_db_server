Data migration scripts.

1. `cd` to the directory of the script and

`GOOS=linux GOARCH=amd64 go build`

2. `kubectl cp` the executable to the cluster

3. `kubectl exec -it mycontainer bash`

5. Run the script `./my-script` in the container.
