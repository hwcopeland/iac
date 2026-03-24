#!/usr/bin/env bash

DIR_HERE=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

# ls "$DIR_HERE/airflow"

DIR_AIRFLOW="$DIR_HERE/airflow"

#
# Clean airflow directory.
#
echo -e 'Clean testing environment'
sudo rm -rf "$DIR_AIRFLOW"

#
# Create working directories
#
echo -e 'Create initial folders\n'
mkdir "$DIR_AIRFLOW"
mkdir "$DIR_AIRFLOW/config"
mkdir "$DIR_AIRFLOW/dags"
mkdir "$DIR_AIRFLOW/logs"
mkdir "$DIR_AIRFLOW/plugins"

#
# Setup docker
#
echo -e 'Setup the local airflow container.\n'
cd "$DIR_AIRFLOW"
curl -LfO 'https://airflow.apache.org/docs/apache-airflow/2.10.2/docker-compose.yaml'
cd "$DIR_HERE"

echo -e 'Create docker environment variables.'
touch "$DIR_AIRFLOW/.env"
echo "AIRFLOW_UID=$(id -u $(whoami))" >> "$DIR_AIRFLOW/.env"

#
# Setup project files.
#
echo -e 'Copy the dags from github\n'
cd "$DIR_AIRFLOW/dags"
rm -f autodock.py
wget https://raw.githubusercontent.com/hwcopeland/airflow-dags/refs/heads/main/autodock.py
cd "$DIR_HERE"

echo -e 'Copy the python code from the project source'

#
# Run airflow
#
echo -e 'spin up the docker containers.\n'
cd "$DIR_AIRFLOW"
sleep 3
# source .env
echo "create the containers"
docker compose create

echo "start the docker service"
docker compose start
# $(docker compose start -- --build)


DOCKER_SERVICE_STARTED=0
BACKOFF_TIME=5
for i in $(seq 1 10);
do
  # SLEEP_TIME = "$i"
  BACKOFF_TIME=$(($BACKOFF_TIME + $i))
  sleep "$BACKOFF_TIME"
  echo "Polling for docker service start. Attempt[$i] = $BACKOFF_TIME"
  DOCKER_PS_WORKER_1=$(docker ps --filter name=airflow-airflow-worker-1 --format json | jq '.Status')
  echo $DOCKER_PS_WORKER_1;
  if [[ $DOCKER_PS_WORKER_1 == *"(healthy)"* ]]; then
    echo -e "Docker has started\n"
    DOCKER_SERVICE_STARTED=1
    break 1
  fi
done

if [[ $DOCKER_SERVICE_STARTED -eq 0 ]]; then
  echo -e "\nFailed to start the docker service.\n"
  docker compose down
fi

#
# Poll for the service to be up and stable
#
echo 'wait for docker containers to be available.'
echo 'execute a task execution.'


#
# Cleanup the airflow
#
cd $DIR_AIRFLOW;
echo -e "Stopping the docker service.\n"
docker compose down