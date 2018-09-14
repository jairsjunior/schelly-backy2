FROM golang:1.10 AS BUILD

#doing dependency build separated from source build optimizes time for developer, but is not required
#install external dependencies first
# ADD go-plugins-helpers/Gopkg.toml $GOPATH/src/go-plugins-helpers/
ADD /main.go $GOPATH/src/schelly-backy2/main.go
RUN go get -v schelly-backy2

#now build source code
ADD schelly-backy2 $GOPATH/src/schelly-backy2
RUN go get -v schelly-backy2


FROM flaviostutz/ceph-client

RUN apt-get update && apt-get -y install python3-alembic python3-dateutil python3-prettytable python3-psutil python3-setproctitle python3-shortuuid python3-sqlalchemy
RUN wget https://github.com/wamdam/backy2/releases/download/v2.9.17/backy2_2.9.17_all.deb -O /backy2.deb
RUN dpkg -i backy2.deb
RUN rm /backy2.deb

VOLUME [ "/backup-source" ]
VOLUME [ "/var/lib/backy2" ] 

EXPOSE 7070

# ENV RESTIC_PASSWORD ''
ENV LISTEN_PORT 7070
ENV LISTEN_IP '0.0.0.0'
ENV LOG_LEVEL 'debug'

#source Ceph RBD image to be backup (rbd://<pool>/<imagename>[@<snapshotname>]) OR
#source file to be backup (file:///backup-source/TESTFILE)
ENV SOURCE_DATA_PATH ''

#file (will store in /var/lib/backy2/data)
#s3 (must be configured with ENVs below)
ENV TARGET_DATA_BACKEND 'file'

ENV S3_AWS_ACCESS_KEY_ID ''
ENV S3_AWS_SECRET_ACCESS_KEY ''
ENV S3_AWS_HOST ''
ENV S3_AWS_PORT '443'
ENV S3_AWS_HTTPS 'true'
ENV S3_AWS_BUCKET_NAME ''
ENV SIMULTANEOUS_WRITES '3'
ENV MAX_BANDWIDTH_WRITE '0'
ENV SIMULTANEOUS_READS '10'
ENV MAX_BANDWIDTH_READ '0'
ENV PROTECT_YOUNG_BACKUP_DAYS '6'

ENV PRE_POST_TIMEOUT '7200'
ENV PRE_BACKUP_COMMAND ''
ENV POST_BACKUP_COMMAND ''

# RUN ln -sf /dev/stdout /var/log/backy.log
RUN touch /var/log/backy.log

COPY --from=BUILD /go/bin/* /bin/
ADD startup.sh /
ADD backy.cfg.template /

CMD [ "/startup.sh" ]
