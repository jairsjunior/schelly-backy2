#!/bin/bash
set +e
set +x

echo "PRE BACKUP SCRIPT (replace this)"
dd if=/dev/zero of=/backup-source/TESTFILE bs=100MB count=1
# echo "dummy test" > /backup-source/TESTFILE

