// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

#define _GNU_SOURCE
#include <arpa/inet.h>
#include <errno.h>
#include <fcntl.h>
#include <stddef.h>
#include <stdio.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/syscall.h>
#include <sys/un.h>
#include <sys/wait.h>
#include <unistd.h>

static int inet_probe(int type, int port) {
    int fd = socket(AF_INET, type, 0);
    if (fd < 0) return -errno;
    struct sockaddr_in addr = {.sin_family=AF_INET, .sin_port=htons(port), .sin_addr.s_addr=htonl(INADDR_LOOPBACK)};
    int result;
    if (type == SOCK_DGRAM) result = sendto(fd, "QA", 2, 0, (struct sockaddr*)&addr, sizeof(addr));
    else result = connect(fd, (struct sockaddr*)&addr, sizeof(addr));
    int status = result < 0 ? -errno : 0;
    close(fd); return status;
}

static int unix_probe(const char *name, int abstract) {
    int fd = socket(AF_UNIX, SOCK_STREAM, 0);
    if (fd < 0) return -errno;
    struct sockaddr_un addr = {.sun_family=AF_UNIX};
    size_t len = strlen(name);
    if (len + abstract >= sizeof(addr.sun_path)) { close(fd); return -ENAMETOOLONG; }
    memcpy(addr.sun_path + abstract, name, len + !abstract);
    int result = connect(fd, (struct sockaddr*)&addr, offsetof(struct sockaddr_un,sun_path)+len+1);
    int status = result < 0 ? -errno : 0;
    close(fd); return status;
}

__attribute__((constructor)) static void isolation_probe(void) {
    int fd = open(QA_INPUT, O_RDONLY|O_CLOEXEC);
    int read_status = fd < 0 ? -errno : 0;
    if(fd >= 0) { char c=0; if(read(fd,&c,1)!=1 || c!='Q') read_status=-EIO; close(fd); }
    fd = open(QA_MARKER, O_WRONLY|O_CREAT|O_EXCL|O_CLOEXEC, 0600);
    int write_status = fd < 0 ? -errno : 0;
    if(fd >= 0) { write(fd,"QA",2); close(fd); }
    int tcp=inet_probe(SOCK_STREAM, QA_TCP_PORT), udp=inet_probe(SOCK_DGRAM, QA_UDP_PORT);
    int pathname=unix_probe(QA_UNIX_PATH,0), abstract=unix_probe(QA_ABSTRACT_NAME,1);
    char *const args[] = {"missing-exec-QA",NULL};
    syscall(SYS_execve, QA_MISSING_EXEC, args, NULL);
    int exec_status=-errno;
    int fork_status=0, exec_child=-1;
    pid_t child=fork();
    if(child<0) fork_status=-errno;
    else if(child==0) { execl("/usr/bin/true","true",(char*)NULL); _exit(101); }
    else { int status=0; if(waitpid(child,&status,0)==child && WIFEXITED(status)) exec_child=WEXITSTATUS(status); }
    // Only this explicitly granted QA file receives output; never stdout/stderr.
    fd=open(QA_RESULT,O_WRONLY|O_CREAT|O_EXCL|O_CLOEXEC,0600);
    if(fd>=0) {
        char buffer[512];
        int count=snprintf(buffer,sizeof(buffer),"{\"read\":%d,\"write\":%d,\"tcp\":%d,\"udp\":%d,\"unixPath\":%d,\"unixAbstract\":%d,\"exec\":%d,\"fork\":%d,\"execChild\":%d}\n",read_status,write_status,tcp,udp,pathname,abstract,exec_status,fork_status,exec_child);
        if(count>0 && count<(int)sizeof(buffer)) write(fd,buffer,count);
        close(fd);
    }
#ifdef QA_HANG
    // Deliberately blocked native constructor: cancellation must kill the
    // worker namespace rather than wait for a cooperative PKCS#11 return.
    for (int i=0;i<30;i++) sleep(1);
#endif
}

// Loading is deliberately unsuccessful after the constructor. No certificate,
// session, PIN or signing operation can be reached by this synthetic module.
unsigned long C_GetFunctionList(void **functions) { *functions=NULL; return 5; }
