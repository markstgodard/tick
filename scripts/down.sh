for run in {1..20}
do
  app=tick$run
  echo $app
  cf d $app -f
done
