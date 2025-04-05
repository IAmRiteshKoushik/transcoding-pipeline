az container create \
  --resource-group myResourceGroup \
  --name transcode-480p \
  --image $ACR_LOGIN_SERVER/transcoder-pipeline:latest \
  --registry-login-server $ACR_LOGIN_SERVER \
  --registry-username $(az acr credential show --name $ACR_NAME --query username -o tsv) \
  --registry-password Z+AWpQvk6GCj5i5BKviRTloaXGQvL0SFx5dt+2mTQM+ACRBBsYAE\
  --environment-variables \
    QUEUE_NAME=transcode-480p-queue \
    RESOLUTION=480 \
    AZURE_STORAGE_CONNECTION_STRING="$AZURE_STORAGE_CONNECTION_STRING" \
  --cpu 1 \
  --memory 1.5 \
  --restart-policy Always \
  --location centralindia \
  --os-type Linux
