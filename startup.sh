#!/bin/bash
set +e
# set +x

echo "Preparing Backy2..."
if [ -f /var/lib/backy2/backy.sqlite ]; then
    echo "Initializing Backy DB..."
    backy2 initdb
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
    --source-path=$SOURCE_DATA_PATH \
    --max-backup-time-running=$MAX_BACKUP_TIME_RUNNING_SECONDS \
    --pre-backup-command=$PRE_BACKUP_COMMAND \
    --post-backup-command=$POST_BACKUP_COMMAND

