FROM debian:12

RUN apt-get update && apt-get install -y curl                          # want "to keep the image small"
RUN apt-get install --no-install-recommends -y curl
RUN apt-get install -y --no-install-recommends curl
RUN apt-get update
RUN echo "hello"
