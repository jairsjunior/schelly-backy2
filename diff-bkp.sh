#!/bin/bash

function initial_backup {
    # call: initial_backup rbd vm1
    POOL="$1"
    VM="$2"

    SNAPNAME=$(date "+%Y-%m-%dT%H:%M:%S")  # 2017-04-19T11:33:23
    TEMPFILE=$(tempfile)

    echo "Performing initial backup of $POOL/$VM."

    rbd snap create "$POOL"/"$VM"@"$SNAPNAME"
    rbd diff --whole-object "$POOL"/"$VM"@"$SNAPNAME" --format=json > "$TEMPFILE"
    backy2 backup -s "$SNAPNAME" -r "$TEMPFILE" rbd://"$POOL"/"$VM"@"$SNAPNAME" $VM

    rm $TEMPFILE
}

function differential_backup {
    # call: differential_backup rbd vm1 old_rbd_snap old_backy2_version
    POOL="$1"
    VM="$2"
    LAST_RBD_SNAP="$3"
    BACKY_SNAP_VERSION_UID="$4"

    SNAPNAME=$(date "+%Y-%m-%dT%H:%M:%S")  # 2017-04-20T11:33:23
    TEMPFILE=$(tempfile)

    echo "Performing differential backup of $POOL/$VM from rbd snapshot $LAST_RBD_SNAP and backy2 version $BACKY_SNAP_VERSION_UID."

    rbd snap create "$POOL"/"$VM"@"$SNAPNAME"
    rbd diff --whole-object "$POOL"/"$VM"@"$SNAPNAME" --from-snap "$LAST_RBD_SNAP" --format=json > "$TEMPFILE"
    # delete old snapshot
    rbd snap rm "$POOL"/"$VM"@"$LAST_RBD_SNAP"
    # and backup
    backy2 backup -s "$SNAPNAME" -r "$TEMPFILE" -f "$BACKY_SNAP_VERSION_UID" rbd://"$POOL"/"$VM"@"$SNAPNAME" "$VM"
}

function backup {
    # call as backup rbd vm1
    POOL="$1"
    VM="$2"

    # find the latest snapshot name from rbd
    LAST_RBD_SNAP=$(rbd snap ls "$POOL"/"$VM"|tail -n +2|awk '{ print $2 }'|sort|tail -n1)
    if [ -z $LAST_RBD_SNAP ]; then
        echo "No previous snapshot found, reverting to initial backup."
        initial_backup "$POOL" "$VM"
    else
        # check if this snapshot exists in backy2
        BACKY_SNAP_VERSION_UID=$(backy2 -m ls -s "$LAST_RBD_SNAP" "$VM"|grep -e '^version'|awk -F '|' '{ print $7 }')
        if [ -z $BACKY_SNAP_VERSION_UID ]; then
            echo "Existing rbd snapshot not found in backy2, reverting to initial backup."
            initial_backup "$POOL" "$VM"
        else
            differential_backup "$POOL" "$VM" "$LAST_RBD_SNAP" "$BACKY_SNAP_VERSION_UID"
        fi
    fi
}

if [ -z $1 ] || [ -z $2 ]; then
        echo "Usage: $0 [pool] [image]"
        exit 1
else
        rbd snap ls "$1"/"$2" > /dev/null 2>&1
        if [ "$?" != "0" ]; then
                echo "Cannot find rbd image $1/$2."
                exit 2
        fi
        backup "$1" "$2"
fi
