#!/bin/bash
set +e
# set +x

# echo "Check S3proxy is up"
# while [ "$(nc -z $S3_AWS_HOST $S3_AWS_PORT </dev/null; echo $?)" !=  "0" ];
# do sleep 5;
# echo "Waiting for S3proxy is UP and RESPONDING";
# done
# sleep 15;

if [[ $INDEX_DB_ADDRESS = *postgres* ]]
then
    [[ $INDEX_DB_ADDRESS =~ :\/\/(.*):(.*)@(.*):([0-9]*)\/?(.*) ]]
    echo "Check Postgres is up"
    echo "POSTGRES_HOST: ${BASH_REMATCH[3]}"
    echo "POSTGRES_PORT: ${BASH_REMATCH[4]}"
    while [ "$(nc -w 5 -z ${BASH_REMATCH[3]} ${BASH_REMATCH[4]} </dev/null; echo $?)" !=  "0" ];
    do sleep 5;
        echo "Waiting for Postgres is UP and RESPONDING";
    done
    sleep 5;
fi

# echo "Preparing Backy2..."
# if [ -f /var/lib/backy2/backy.sqlite ]; then
#     echo "Initializing Backy DB..."
#     backy2 initdb
# fi

BACKY2LS=$(backy2 ls)
if [[ $BACKY2LS = *"Please run initdb first"* ]]
then
    echo "== Initializing Backy DB =="
    backy2 initdb
else
    echo "== Backy2 DB is inited! =="
fi

cat /backy.cfg.template | envsubst > /etc/backy.cfg
cat /etc/backy.cfg

#redirect backy2 logs to stdout and remove internal log file to avoid increasing endlessly
tail -f /var/log/backy.log&
while true; do if [ -f /var/log/backy.log ]; then rm /var/log/backy.log; fi; sleep 86400; done&

echo "Starting Backy2 API..."
schelly-backy2 \
    --listen-ip=$LISTEN_IP \
    --listen-port=$LISTEN_PORT \
    --log-level=$LOG_LEVEL \
    --source-path="$SOURCE_DATA_PATH" \
    --pre-post-timeout=$PRE_POST_TIMEOUT \
    --pre-backup-command="$PRE_BACKUP_COMMAND" \
    --post-backup-command="$POST_BACKUP_COMMAND"

