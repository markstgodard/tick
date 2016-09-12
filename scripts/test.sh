# push, no start
for run in {1..3}
do
  date
  app=tick$run
  echo $app
  cf push $app --no-start -i 2 -m 128m
  cf set-env $app REGISTRY_HOST "10.255.67.67:8080"
  cf access-allow $app registry --protocol tcp --port 8080
done

# enable access app to app
for x in {1..3}
do
  app=tick$x
  for y in {1..3}
  do
    app2=tick$y
    cf access-allow $app $app2 --protocol tcp --port 8080
  done
done

# start apps
for run in {1..3}
do
  date
  app=tick$run
  echo $app
  cf start $app
done
