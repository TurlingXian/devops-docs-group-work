/*
 * Exercise on thread synchronization.
 *
 * Assume a half-duplex communication bus with limited capacity, measured in
 * tasks, and 2 priority levels:
 *
 * - tasks: A task signifies a unit of data communication over the bus
 *
 * - half-duplex: All tasks using the bus should have the same direction
 *
 * - limited capacity: There can be only 3 tasks using the bus at the same time.
 *                     In other words, the bus has only 3 slots.
 *
 *  - 2 priority levels: Priority tasks take precedence over non-priority tasks
 *
 *  Fill-in your code after the TODO comments
 */

#include <stdio.h>
#include <string.h>

#include "tests/threads/tests.h"
#include "threads/malloc.h"
#include "threads/thread.h"
#include "timer.h"

/* This is where the API for the condition variables is defined */
#include "threads/synch.h"

/* This is the API for random number generation.
 * Random numbers are used to simulate a task's transfer duration
 */
#include "lib/random.h"

#define MAX_NUM_OF_TASKS 200

#define BUS_CAPACITY 3

#define MIN(a, b) a < b ? a : b

typedef enum {
  SEND,
  RECEIVE,

  NUM_OF_DIRECTIONS
} direction_t;

typedef enum {
  NORMAL,
  PRIORITY,

  NUM_OF_PRIORITIES
} priority_t;

typedef struct {
  direction_t direction;
  priority_t priority;
  unsigned long transfer_duration;
} task_t;

void init_bus (void);
void batch_scheduler (unsigned int num_priority_send,
                      unsigned int num_priority_receive,
                      unsigned int num_tasks_send,
                      unsigned int num_tasks_receive);

void wait_to_be_onboard(unsigned int *total_waiting_count, struct semaphore *sem);
int wakeup_waiting_task(struct semaphore *sem, unsigned int waiting);

/* Thread function for running a task: Gets a slot, transfers data and finally
 * releases slot */
static void run_task (void *task_);

/* WARNING: This function may suspend the calling thread, depending on slot
 * availability */
static void get_slot (const task_t *task);

/* Simulates transfering of data */
static void transfer_data (const task_t *task);

/* Releases the slot */
static void release_slot (const task_t *task);

static struct semaphore bus_slots;
static struct semaphore mutex;
static struct semaphore has_sender_semaphore;
static struct semaphore has_receiver_semaphore;
static struct semaphore has_sender_priority_semaphore;
static struct semaphore has_receiver_priority_semaphore;

static int bus_direction;
static int task_on_bus;
static int waiting_task;
static int send_task;
static int receiver_task;
static int send_priority_task;
static int recv_priority_task;

void init_bus (void) {

  random_init ((unsigned int)123456789);

  /* TODO: Initialize global/static variables,
     e.g. your condition variables, locks, counters etc */
  sema_init(&bus_slots, BUS_CAPACITY);
  sema_init(&mutex, 1);
  sema_init(&has_sender_semaphore, 0);
  sema_init(&has_receiver_semaphore, 0);
  sema_init(&has_sender_priority_semaphore, 0);
  sema_init(&has_receiver_priority_semaphore, 0);

  bus_direction = -1; // not started yet
  task_on_bus = 0;
  waiting_task = 0;
  send_task = 0;
  receiver_task = 0;
  send_priority_task = 0;
  recv_priority_task = 0;
}

void batch_scheduler (unsigned int num_priority_send,
                      unsigned int num_priority_receive,
                      unsigned int num_tasks_send,
                      unsigned int num_tasks_receive) {
  ASSERT (num_tasks_send + num_tasks_receive + num_priority_send +
             num_priority_receive <= MAX_NUM_OF_TASKS);

  static task_t tasks[MAX_NUM_OF_TASKS] = {0};

  char thread_name[32] = {0};

  unsigned long total_transfer_dur = 0;

  int j = 0;

  /* create priority sender threads */
  for (unsigned i = 0; i < num_priority_send; i++) {
    tasks[j].direction = SEND;
    tasks[j].priority = PRIORITY;
    tasks[j].transfer_duration = random_ulong() % 244;

    total_transfer_dur += tasks[j].transfer_duration;

    snprintf (thread_name, sizeof thread_name, "sender-prio");
    thread_create (thread_name, PRI_DEFAULT, run_task, (void *)&tasks[j]);

    j++;
  }

  /* create priority receiver threads */
  for (unsigned i = 0; i < num_priority_receive; i++) {
    tasks[j].direction = RECEIVE;
    tasks[j].priority = PRIORITY;
    tasks[j].transfer_duration = random_ulong() % 244;

    total_transfer_dur += tasks[j].transfer_duration;

    snprintf (thread_name, sizeof thread_name, "receiver-prio");
    thread_create (thread_name, PRI_DEFAULT, run_task, (void *)&tasks[j]);

    j++;
  }

  /* create normal sender threads */
  for (unsigned i = 0; i < num_tasks_send; i++) {
    tasks[j].direction = SEND;
    tasks[j].priority = NORMAL;
    tasks[j].transfer_duration = random_ulong () % 244;

    total_transfer_dur += tasks[j].transfer_duration;

    snprintf (thread_name, sizeof thread_name, "sender");
    thread_create (thread_name, PRI_DEFAULT, run_task, (void *)&tasks[j]);

    j++;
  }

  /* create normal receiver threads */
  for (unsigned i = 0; i < num_tasks_receive; i++) {
    tasks[j].direction = RECEIVE;
    tasks[j].priority = NORMAL;
    tasks[j].transfer_duration = random_ulong() % 244;

    total_transfer_dur += tasks[j].transfer_duration;

    snprintf (thread_name, sizeof thread_name, "receiver");
    thread_create (thread_name, PRI_DEFAULT, run_task, (void *)&tasks[j]);

    j++;
  }

  /* Sleep until all tasks are complete */
  timer_sleep (2 * total_transfer_dur);
}

/* Thread function for the communication tasks */
void run_task(void *task_) {
  task_t *task = (task_t *)task_;

  get_slot (task);

  msg ("%s acquired slot", thread_name());
  transfer_data (task);

  release_slot (task);
}

static direction_t other_direction(direction_t this_direction) {
  return this_direction == SEND ? RECEIVE : SEND;
}

void get_slot (const task_t *task) {

  /* TODO: Try to get a slot, respect the following rules:
   *        1. There can be only BUS_CAPACITY tasks using the bus
   *        2. The bus is half-duplex: All tasks using the bus should be either
   * sending or receiving
   *        3. A normal task should not get the bus if there are priority tasks
   * waiting
   *
   * You do not need to guarantee fairness or freedom from starvation:
   * feel free to schedule priority tasks of the same direction,
   * even if there are priority tasks of the other direction waiting
   */
  sema_down(&mutex);
  bool must_wait = false;
  if(task_on_bus >= BUS_CAPACITY){
    must_wait = true;
  }
  else if (bus_direction != -1 && bus_direction != task->direction){
    must_wait = true;
  }
  else if (task->priority == NORMAL){
    /* code */
    if(task->direction == SEND && send_priority_task > 0){
      must_wait = true;
    }
    else if (task->direction == RECEIVE && recv_priority_task > 0){
      /* code */
      must_wait = true;
    }
  }

  if(must_wait){
    if(task->priority == PRIORITY){
      if (task->direction == SEND)
      {
        /* code */
        wait_to_be_onboard(&send_priority_task, &has_sender_priority_semaphore);
      }
      else
      {
        wait_to_be_onboard(&recv_priority_task, &has_receiver_priority_semaphore);
      }
    }
    else
    {
      if (task->direction == SEND)
      {
        /* code */
        wait_to_be_onboard(&send_task, &has_sender_semaphore);
      }
      else
      {
        wait_to_be_onboard(&receiver_task, &has_receiver_semaphore);
      }
    }
  }
  
  sema_down(&bus_slots);
  task_on_bus++;

  bus_direction = task->direction;
  sema_up(&mutex);
}

void transfer_data (const task_t *task) {
  /* Simulate bus send/receive */
  timer_sleep (task->transfer_duration);
}

void release_slot (const task_t *task) {

  /* TODO: Release the slot, think about the actions you need to perform:
   *       - Do you need to notify any waiting task?
   *       - Do you need to increment/decrement any counter?
   */
  // enter critical selection
  sema_down(&mutex);
  task_on_bus--;

  sema_up(&bus_slots);
  if(task_on_bus == 0){
    bus_direction = -1; //idle
  }

  if(bus_direction != RECEIVE){
    int can_aboard = wakeup_waiting_task(&has_sender_priority_semaphore, send_priority_task);
    if(recv_priority_task == 0){
      can_aboard += wakeup_waiting_task(&has_sender_semaphore, send_task);
    }
    if(can_aboard > 0){
      bus_direction = SEND;
    }
  }

  if(bus_direction != SEND){
    int can_aboard = wakeup_waiting_task(&has_receiver_priority_semaphore, recv_priority_task);
    if(send_priority_task == 0){
      can_aboard += wakeup_waiting_task(&has_receiver_semaphore, receiver_task);
    }
    if(can_aboard > 0){
      bus_direction = RECEIVE;
    }
  }

  sema_up(&mutex);
}

void wait_to_be_onboard(unsigned int *total_waiting_count, struct semaphore *sem){
    *total_waiting_count += 1;

    /* Allow other threads to continue getting/leaving/processing data */
    sema_up(&mutex);

    /* When there is a slot available a valid semaphore will incremented */
    sema_down(sem);

    /* The calling function is expected to have the mutex. Reacquire it. */
    sema_down(&mutex);

    /* We are no longer waiting for a slot */
    *total_waiting_count -= 1;
    waiting_task -= 1;
}

int wakeup_waiting_task(struct semaphore *sem, unsigned int waiting){
    int empty_bus_slots = BUS_CAPACITY - task_on_bus - waiting_task;
    int to_wake = MIN(empty_bus_slots, waiting);

    for (int i = 0; i < to_wake; i++)
    {
        sema_up(sem);
        waiting_task++;
    }
    return to_wake;
}

