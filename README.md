# About Concept02

Concept02 is k8s native application written in Go which is meant to allow cluster users to schedule workloads in a better way that core k8s allows.

**Currently the project is in concept spike phase** More documentation will be provided as the project evolves

## Deployment Notes
Concept02 is currently can be executed both from outside the cluster (using kubectl configuration) or from within the cluster.

## Development Notes

### Building Go binary
No special instructions just run a regular go build command to create the `concept02` binary
`go build`

### Building Dockerfile 
Simply run the following command from the project's root dir
`docker build . --tag concept02:dev`