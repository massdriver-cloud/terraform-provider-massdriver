# Grants have no get-by-ID API, so import takes the composite
# <repository_id>/<grant_id> form.
terraform import massdriver_oci_repository_grant.pull_for_eng <repository_id>/<grant_id>
