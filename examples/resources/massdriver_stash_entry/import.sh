# The import ID is <kind>:<scope-id>/<key>, where kind is project,
# environment, component, instance, or deployment. The API does not return
# `ttl`, or a sensitive value that the caller cannot read, so when the
# configuration sets them, the next apply writes the entry again. For an
# immutable entry, that write deletes the entry first.
terraform import massdriver_stash_entry.links environment:ecomm-prod/md/v1/links
