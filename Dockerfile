ARG base_image
FROM ${base_image}

COPY ./ecr-creds-rotate /opt

WORKDIR /opt
RUN ls

ENTRYPOINT ["/opt/ecr-creds-rotate"]