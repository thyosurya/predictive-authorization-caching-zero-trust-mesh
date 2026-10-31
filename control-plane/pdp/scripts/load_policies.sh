#!/bin/bash
echo "Loading OPA policies..."
curl -X PUT --data-binary @control-plane/pdp/policies/authz.rego http://localhost:8181/v1/policies/authz
curl -X PUT --data-binary @control-plane/pdp/policies/data.json http://localhost:8181/v1/data/authz
echo "Policies loaded successfully."
