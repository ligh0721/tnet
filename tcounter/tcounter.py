import socket
import struct

cmd_send_value = 0

class CounterClient:
    Addr = ""
    __conn = None
    __struct = None
    def __init__(self):
        self.__conn = socket.socket(socket.AF_UNIX, socket.SOCK_DGRAM)
        self.__struct = struct.Struct('>IIq')

    def Close(self):
        self.__conn.close()

    def SendValue(self, key, value):
        encoded = self.__struct.pack(cmd_send_value, key, value)
        self.__conn.sendto(encoded, self.Addr)

