FROM debian:stretch-slim

RUN apt-get update && apt-get install git wget unzip -y && \
    wget https://github.com/mailslurper/mailslurper/releases/download/1.14.1/mailslurper-1.14.1-linux.zip && \
    unzip mailslurper-1.14.1-linux.zip

COPY config.json ./

ENTRYPOINT ["./mailslurper"]
