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

### Setting up localstack resources

To create an SQS queue, an SNS topic and a topic subscription from SNS to SQS, which mirrors our infrastructure in production, run the follwing command

```shell
make local.setup
```

### Build the provider and install the terraform plugin

In order to run local versions of the provider we have to manually install it in our terraform plugins directory. To do this run

```shell
make install
```

### Test sample configuration

There are two ways to test the terraform provider end to end. The first is to run acceptance tests using the framework
that provided by Hashicorp. To run the tests it is helpful to open a split pane in your terminal. In one pane run the following command which will poll the sqs created in `local.setup` and display any messages received.

```shell
make local.sqs.poll
```

In the other pane run

```shell
make testacc
```

The second variation uses actual Terraform configuration to test the provider. In `examples/main.tf` you will find a Terraform resource file where you can stub inputs and use your resource as it would be in someone elses configuration. Using the split pane method above, use the `local.sqs.poll` command in one pane. In the other run

```shell
make local.apply
```
