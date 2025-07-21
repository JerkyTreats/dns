# Use official CoreDNS image
FROM coredns/coredns:1.11.1

# Copy Corefile and zone definitions
COPY configs/coredns/Corefile /etc/coredns/Corefile
COPY configs/coredns/zones/ /zones/

# Default command to run CoreDNS with the Corefile
CMD ["-conf", "/etc/coredns/Corefile"]
