FROM golang:1.12.3 AS BUILD

RUN mkdir /schelly-backy2
WORKDIR /schelly-backy2

ADD go.mod .
ADD go.sum .
RUN go mod download

#now build source code
ADD schelly-backy2/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o /go/bin/schelly-backy2 .


FROM flaviostutz/ceph-client:13.2.0.3

RUN apt-get update && apt-get -y install python3-alembic python3-dateutil python3-prettytable python3-psutil python3-setproctitle python3-shortuuid python3-sqlalchemy python3-psycopg2 netcat 
RUN DEBIAN_FRONTEND=noninteractive apt-get install -y python3-boto python3-azure-storage
RUN apt-get install -y librados-dev librbd-dev rbd-nbd nbd-client 

RUN wget https://github.com/jairsjunior/backy2/raw/master/dist/backy2_2.9.18_all.deb -O /backy2.deb
RUN dpkg -i backy2.deb
RUN rm /backy2.deb
RUN apt-get -y -f install

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

ENV AZURE_ACCESS_KEY_ID ''
ENV AZURE_SECRET_ACCESS_KEY ''
ENV AZURE_BUCKET_NAME ''

ENV SIMULTANEOUS_WRITES '3'
ENV MAX_BANDWIDTH_WRITE '0'
ENV SIMULTANEOUS_READS '10'
ENV MAX_BANDWIDTH_READ '0'
ENV PROTECT_YOUNG_BACKUP_DAYS '6'

ENV PRE_POST_TIMEOUT '7200'
ENV PRE_BACKUP_COMMAND ''
ENV POST_BACKUP_COMMAND ''
ENV INDEX_DB_ADDRESS 'sqlite:////var/lib/backy2/backy.sqlite'

# RUN ln -sf /dev/stdout /var/log/backy.log
RUN touch /var/log/backy.log

COPY --from=BUILD /go/bin/* /bin/
ADD startup.sh /
ADD backy.cfg.template /
ADD diff-bkp.sh /
RUN chmod +x diff-bkp.sh

CMD [ "/startup.sh" ]
