# Grants have no get-by-ID API, so import takes the composite
# <resource_id>/<grant_id> form.
terraform import massdriver_resource_grant.share_with_eng <resource_id>/<grant_id>
