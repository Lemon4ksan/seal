CC ?= gcc
CFLAGS ?= -std=c11 -O3 -Wall -Wextra -Wno-unused-parameter -I.

TARGET = test_przm_dec.exe
LIB = libprzm_dec.a

all: $(TARGET) $(LIB)

$(TARGET): test_przm_dec.c przm_dec.c przm_dec.h
	$(CC) $(CFLAGS) test_przm_dec.c przm_dec.c -o $(TARGET)

$(LIB): przm_dec.o
	ar rcs $(LIB) przm_dec.o

przm_dec.o: przm_dec.c przm_dec.h
	$(CC) $(CFLAGS) -c przm_dec.c -o przm_dec.o

test: $(TARGET)
	./$(TARGET)

clean:
	rm -f $(TARGET) $(LIB) *.o

.PHONY: all test clean
