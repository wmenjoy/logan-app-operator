FROM registry.access.redhat.com/rhel7:latest as rhel7
FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

# copy zoneinfo from rhel7 to ubi-minimal
COPY  --from=rhel7 /usr/share/zoneinfo /usr/share/zoneinfo

ENV OPERATOR=/usr/local/bin/logan-app-operator \
    USER_UID=1001 \
    USER_NAME=logan-app-operator \
    TZ=Asia/Shanghai

# install operator binary
COPY build/_output/bin/logan-app-operator ${OPERATOR}

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
