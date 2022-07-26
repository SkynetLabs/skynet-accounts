#!/bin/bash

MONGO_USER=admin
MONGO_PASSWORD=aO4tV5tC1oU3oQ7u
MONGO_PORT=37017
MONGO_TEST_CONTAINER_NAME=$1
MONGO_REPLSET=skynet

# Stop and remove any existing docker container
printf '\n==STOPPING AND REMOVING DOCKER CONTAINERS==\n'
docker stop $MONGO_TEST_CONTAINER_NAME 1>/dev/null 2>&1
docker rm $MONGO_TEST_CONTAINER_NAME 1>/dev/null 2>&1

# Start docker container
printf '\n==STARTING DOCKER CONTAINER==\n'
docker run \
	--rm \
	--detach \
	--name $MONGO_TEST_CONTAINER_NAME \
	-p $MONGO_PORT:$MONGO_PORT \
	-e MONGO_INITDB_ROOT_USERNAME=$MONGO_USER \
	-e MONGO_INITDB_ROOT_PASSWORD=$MONGO_PASSWORD \
	mongo:4.4.1 mongod --port=$MONGO_PORT --replSet=$MONGO_REPLSET 1>/dev/null 2>&1

# wait for mongo to start before we try to configure it
printf '\n==WAIT FOR MONGO TO BE ACCESSIBLE==\n'
status=1
while [ $status -gt 0 ]; do
	sleep 1
	# Execute command and save the stderr
	err="$(docker exec $MONGO_TEST_CONTAINER_NAME mongo -u $MONGO_USER -p $MONGO_PASSWORD --port $MONGO_PORT 2>&1)"
	# Grab the status code
	status=$?
	# Log for debugging
	echo $err
	echo $status
done

# Initialise a single node replica set.
printf '\n==INITIALIZE REPLICASET==\n'
# Execute command and save the stderr
docker exec $MONGO_TEST_CONTAINER_NAME mongo -u $MONGO_USER -p $MONGO_PASSWORD --port $MONGO_PORT --eval "rs.initiate({_id: \"$MONGO_REPLSET\", members: [{ _id: 0, host: \"localhost:$MONGO_PORT\" }]})"
