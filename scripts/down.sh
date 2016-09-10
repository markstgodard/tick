for run in {1..25}
do
  app=tick$run
  echo $app
  cf d $app -f
done
