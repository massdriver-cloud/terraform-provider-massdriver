# The token is only returned at creation, so an imported service account has
# an empty `token`.
terraform import massdriver_service_account.ci <service_account_id>
