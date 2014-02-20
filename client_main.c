#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/time.h>

#include <netinet/in.h>
#include <netdb.h>

#include <errno.h>
#include <netinet/in.h>
#include <arpa/inet.h>

#include "../headers/client.h"
#include "../headers/defs.h"

char host[MAX_HOST_NAME_LEN];
char nick[MAX_MEMBER_NAME_LEN];

u_int16_t tcpPort;
u_int16_t udpPort;
u_int16_t recPort; 
u_int16_t mID = 0;

struct addrinfo *tcpConn;
struct addrinfo *udpConn;
struct addrinfo *recConn;

int tcpFd;
int udpFd;
int recFd;

/* Generic Chat Message sender
 * Builds a chat message with the given msg data
 * Sends it out on the known tcp port
 */ 
int sendChatMsg(uint16_t dataLen, char *data) {
    char buffer[MAX_MSG_LEN];
    memset(buffer, 0, MAX_MSG_LEN);

    // building chat message 
    struct chat_msghdr *chatData = (struct chat_msghdr *) buffer;
    chatData->sender.member_id = mID; //already in network byte order
    chatData->msg_len = (sizeof(struct chat_msghdr) + dataLen);
    memcpy(chatData->msgdata, data, dataLen);

    // send request to server with addrinfo since udp is a pain
    int err = sendto(udpFd, buffer, chatData->msg_len, 0, udpConn->ai_addr, udpConn->ai_addrlen);
    return err;
}

char * getNameBuffer() {
    return malloc(MAX_MEMBER_NAME_LEN);
}

char * readChatMsg(char *name) {
    char buffer[MAX_MSG_LEN];
    memset(buffer, 0, MAX_MSG_LEN);
    int numRead = recvfrom(recFd, buffer, MAX_MSG_LEN, 0, recConn->ai_addr, &(recConn->ai_addrlen));
    if (numRead <= 0) { return ""; }
    struct chat_msghdr *chatData = (struct chat_msghdr *) buffer;

    char *data = malloc(htons(chatData->msg_len));
    memcpy(name, chatData->sender.member_name, MAX_MEMBER_NAME_LEN);
    memcpy(data, chatData->msgdata, htons(chatData->msg_len));
    return data;
}

/* Generic Control Message Sender
 * Builds a control message with the given message data
 * Message is sent out the known tcp connection. 
 * Connection is closed after completion
 * Return -1 on failure
 */
int sendCtrlMsg(uint16_t msgType, uint16_t dataLen, char *data, char *resp) {
    char buffer[MAX_MSG_LEN];
    memset(buffer, 0, MAX_MSG_LEN);

    // connect to server TCP port
    int tcpFd = socket(tcpConn->ai_family, tcpConn->ai_socktype, tcpConn->ai_protocol);
    
    if (tcpFd == -1)
        return -1;

    int err = connect(tcpFd, tcpConn->ai_addr, tcpConn->ai_addrlen);

    if(err) 
        return err;
    
    // building control message    
    struct control_msghdr *ctrlData = (struct control_msghdr *) buffer;
    ctrlData->msg_type = htons(msgType);
    ctrlData->member_id = mID; 
    ctrlData->msg_len = sizeof (struct control_msghdr) + dataLen;
    memcpy(ctrlData->msgdata, data, dataLen);

    // send request to server
    err = send(tcpFd, buffer, ctrlData->msg_len, 0);
    if(err == -1)
        return -1;

    // receive response from server
    memset(resp, 0, MAX_MSG_LEN);
    int numRecv = recv(tcpFd, resp, MAX_MSG_LEN, 0);

    close(tcpFd);

    return numRecv;
}

/* Register the known client
 * Function should only be needed for the initial connection
 */
int registerClient() {
    char buffer[MAX_MSG_LEN];
    char resp[MAX_MSG_LEN];
    memset(buffer, 0, MAX_MSG_LEN);

    // build the registration request
    struct register_msgdata *regData = (struct register_msgdata *) buffer;
    regData->udp_port = recPort;
    strcpy((char *) regData->member_name, nick);
    uint16_t regLen = sizeof (struct register_msgdata) + strlen(nick) + 1;

    // send registration reuqest, watch for errors
    int err = sendCtrlMsg(REGISTER_REQUEST, regLen, buffer, resp);

    if (err < 0) 
        return -1;

    // parse the response
    struct control_msghdr *ctrlData = (struct control_msghdr *) resp;    

    return ((ctrlData->msg_type == htons(REGISTER_SUCC)) ? (mID = ctrlData->member_id) : -1);
}

/* Get info from the server based on msgtype
 * Function should only be needed for control messages that return
 * simple char streams
 */
char* requestInfo(uint16_t MSG_TYPE, char* data, uint16_t length, uint16_t MSG_SUCC) {
    char *resp = (char *)malloc(MAX_MSG_LEN);

    // send request, watch for errors
    int err = sendCtrlMsg(MSG_TYPE, length, data, resp);
    if (err < 0) 
        return "boo";

    // parse the response
    struct control_msghdr *ctrlData = (struct control_msghdr *) resp;    

    return (char *)ctrlData->msgdata;
    //return ((ctrlData->msg_type == htons(MSG_SUCC)) ? (char *)ctrlData->msgdata : NULL);
}

/*
 * Burn baby burn
 */
void shutdownClient() {
    if(tcpConn)
        freeaddrinfo(tcpConn);
    if(udpConn)
        freeaddrinfo(udpConn);
    if(recConn)
        freeaddrinfo(recConn);

    if(tcpFd)
        close(tcpFd);
    if(udpFd)
        close(udpFd);
    if(recFd)
        close(recFd);
}

int initReciever() {
    
    // setup UDP reciever addrinfo. We connect immediately
    struct addrinfo recInfo;
    memset(&recInfo, 0, sizeof(recInfo));
    recInfo.ai_family = AF_INET;
    recInfo.ai_socktype = SOCK_DGRAM;
    recInfo.ai_flags = AI_PASSIVE;

    // I'm so mad that GO doesnt write out byte streams correctly cause this is bananas 
    int err = getaddrinfo("localhost", "0", &recInfo, &recConn);
    if(err) { return -1; }
    // recFd = socket(recConn->ai_family, recConn->ai_socktype, recConn->ai_protocol);
    recFd = socket(recConn->ai_family, recConn->ai_socktype, 0);

    if (recFd == -1) { return -1; }
    err = bind(recFd, recConn->ai_addr, recConn->ai_addrlen);
    if (err == -1) { return -1; }
    //nothing updates itself in this protocal
    getsockname(recFd, recConn->ai_addr, &(recConn->ai_addrlen));
    recPort = (((struct sockaddr_in *)recConn->ai_addr)->sin_port);
    return htons(recPort);
}
/* Setup the initial connection along with the flags passed in by the higher
 * level go routine. C enviornment should be good after this point
 * Doing sockets and buffered reading in GO turns out to be practically impossible
 */
int initConnections(char *hostName, uint16_t tcp, uint16_t udp, char *name) {
    // get flag information 
    strncpy(host, hostName, MAX_HOST_NAME_LEN);
    tcpPort = tcp;
    udpPort = udp;
    strncpy(nick, name, MAX_MEMBER_NAME_LEN);

    // setup TCP addrinfo. We connect as we need.
    struct addrinfo tcpInfo;
    memset(&tcpInfo, 0, sizeof (tcpInfo));
    tcpInfo.ai_family = AF_INET;
    tcpInfo.ai_socktype = SOCK_STREAM;
    char tcpPortStr[MAX_HOST_NAME_LEN];
    sprintf(tcpPortStr, "%d", tcpPort);
    int err = getaddrinfo(host, tcpPortStr, &tcpInfo, &tcpConn);
    
    if(err) 
        return -1;

    // setup UDP addrinfo. We connect immediately
    struct addrinfo udpInfo;
    memset(&udpInfo, 0, sizeof (udpInfo));
    udpInfo.ai_family = AF_INET;
    udpInfo.ai_socktype = SOCK_DGRAM;
    char udpPortStr[MAX_HOST_NAME_LEN];
    sprintf(udpPortStr, "%d", udpPort);

    err = getaddrinfo(host, udpPortStr, &udpInfo, &udpConn);

    if(err)
        return -1;

    udpFd = socket(udpConn->ai_family, udpConn->ai_socktype, udpConn->ai_protocol);
    if (udpFd == -1)
        return -1;

	return 0;
}