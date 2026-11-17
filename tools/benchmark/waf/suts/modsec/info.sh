docker exec $SUT_NAME nginx -v 2>&1
docker exec $SUT_NAME sh -c "ls /usr/local/modsecurity/lib/libmodsecurity.so.3.0.* 2>/dev/null | sed 's/.*so\./libmodsecurity /'; grep -m1 -o \"ver:'OWASP_CRS/[0-9.]*\" /etc/modsecurity.d/owasp-crs/rules/REQUEST-901-INITIALIZATION.conf"
