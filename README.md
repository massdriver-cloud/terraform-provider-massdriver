# Terraform Provider Massdriver

This provider allows us to manage massdriver artifacts and package alarms generated from bundles right alongside the other resources in the bundle. It gives us delineated lifecycle events (create, update, destroy).

## Development Environment

This project comes with all of the tools you need dockerized in a dev container. Make sure your vscode installation has the plugin [remote-containers](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers).
Use the command palette to run `Remote-Containers: Open Folder In Container` this will install all the dependencies for development and testing as well as start LocalStack.

## GraphQL Code Generation

This provider uses [genqlient](https://github.com/Khan/genqlient) to generate type-safe GraphQL client code from the Massdriver API schema.

### Setup

After cloning the repository:

```shell
go mod download
```

### Adding or Modifying GraphQL Queries

1. Edit `massdriver/genqlient.graphql` to add/modify queries or mutations
2. Regenerate the client code:

```shell
cd massdriver
go generate
cd ..
```

This will update `massdriver/zz_graphql.go` with the generated code.

### Files

- `massdriver/graphql.go` - Contains the `//go:generate` directive
- `massdriver/genqlient.yaml` - genqlient configuration
- `massdriver/genqlient.graphql` - GraphQL operations (queries/mutations)
- `massdriver/schema.graphql` - Massdriver API schema (copied from CLI)
- `massdriver/zz_graphql.go` - Generated GraphQL client code (do not edit)

**Note:** Always run `go generate` after modifying GraphQL queries. The generated code is not automatically updated during `go build`.

## Testing

### Build the provider and install the terraform plugin

In order to run local versions of the provider we have to manually install it in our terraform plugins directory. To do this run

```shell
make install
```
