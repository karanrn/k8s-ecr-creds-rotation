ARG base_image
FROM ${base_image}

RUN pwd

COPY ecr-creds-rotate /opt

ENTRYPOINT ["/opt/ecr-creds-rotate"]