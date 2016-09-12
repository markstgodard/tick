for run in {1..3}
do
  app=tick$run
  echo $app
  cf d $app -f
done
