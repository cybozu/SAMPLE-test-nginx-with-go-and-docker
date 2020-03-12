FROM nginx:1.17.9

COPY nginx.conf.tmpl /etc/nginx/nginx.conf.tmpl
COPY entrypoint /entrypoint

CMD ["/entrypoint"]
