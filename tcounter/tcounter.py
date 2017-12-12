import socket
import struct

cmd_send_value = 0

class CounterClient:
    _conn = None
    _struct = None
    _addr = None
    def __init__(self, addr):
        if type(addr) is str:
            self._conn = socket.socket(socket.AF_UNIX, socket.SOCK_DGRAM)
        elif type(addr) is tuple:
            self._conn = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        else:
            raise TypeError('address must be string(AF_UNIX) or tulpe(AF_INET)')

        self._struct = struct.Struct('>IIq')
        self._addr = addr

    def Close(self):
        self._conn.close()

    def SendValue(self, key, value):
        encoded = self._struct.pack(cmd_send_value, key, value)
        self._conn.sendto(encoded, self._addr)

