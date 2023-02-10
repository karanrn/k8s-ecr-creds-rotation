ARG base_image
FROM ${base_image}

COPY ecr-creds-rotate /opt

ENTRYPOINT ["/opt/ecr-creds-rotate"]