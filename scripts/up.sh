for run in {1..25}
do
  date
  app=tick$run
  echo $app
  cf push $app --no-start -i 5 -m 128m
  cf set-env $app REGISTRY_HOST "10.255.67.67:8080"
  cf access-allow $app registry --protocol tcp --port 8080
  cf start $app

  echo "Number of apps registered"
  curl -s registry.toque.c2c.cf-app.com/api/v1/instances | jq '.instances | length'
done
