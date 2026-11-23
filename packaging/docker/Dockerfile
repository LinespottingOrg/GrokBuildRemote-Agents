FROM debian:stable-slim
COPY gbr-agent /usr/bin/gbr-agent
RUN chmod 755 /usr/bin/gbr-agent
ENTRYPOINT ["gbr-agent"]
CMD ["version"]
