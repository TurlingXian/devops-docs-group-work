#include <dirent.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <getopt.h>

#define MAX_LEN 256
#define OPT_HELP 1
#define OPT_LONG 2
#define OPT_ALL 3
#define OPT_SORT 4
#define SUPPORT_OPT 4

// additional process for printing the dir_stream type
// first, pick the 3 most common types: directory, file and symlink

/**
 * Additional process for printing the dir_stream type.
 * First, pick the 3 most common types: directory, file and symlink.
 * DI_DIR = directory = 4
 * DI_LNK = link = 10
 * DI_REG = regular file = 8
 *
 * @c: The input, which is a unsigned character, as documentation.
 *
 * @return: The letter to represent the directory type.
 */
char additional_dir_type_process(unsigned char c);

/**
 * Collect the result into an array, sort them to make they're...more like the actual "ls".
 * This is a struct.
 */
struct ls_entry
{
    char name[MAX_LEN + 1];
    unsigned char type;
    unsigned long size;
    int is_hidden;
};

/**
 * For sorting the "directories".
 * Pass two pointer, compare their data. Return int result.
 * Using void pointer is not safe, but will be refactored later.
 * @param: Two entry (ls_entry).
 *
 * @return: indicate which comes first.
 */
int cmp(const void *p1, const void *p2);

/**
 * Handler hidden file or directories.
 * Using the d_name to catch.
 * @param: struct ls_entry
 *
 * @return: modifies the property of ls_entry
 */
void check_hidden(struct ls_entry *current_entry);

/**
 * The main feature of this code is the ls - list command that will be taken place here.
 * This command can accept a string indicated the desired directory or none, in this case, it will list the current directory.
 * @param: a string for directory or none.
 * 
 * @return: print the content in the input directory to the stdout - or the screen.
 */
void list(char directory[], int arguments[]);

int main(int argc, char *argv[]){
    int ret;
    // possible option
    char* options = "hlas";
    int arguments[SUPPORT_OPT + 1];
    
    // check man page of getlong_opt man page
    struct option longopts[] = {
        {"help", no_argument, NULL, 'h'},
        {"long", no_argument, NULL, 'l'},
        {"all", no_argument, NULL, 'a'},
        {"short", no_argument, NULL, 's'},
        {NULL, 0, NULL, 0}
    };
    
    while((ret = getopt_long(argc, argv, options, longopts, NULL)) != -1){
        // printf("getopt_long returned: '%c' (%d).\n", ret, ret);
        switch (ret){
        case 'h':
            printf("This is the help message for this program. This program\n");
            printf("has somewhat similar feature as the list-ls UNIX command\n");
            printf("Supported operator are: -h, -s, -l and -a.\n");
            printf("-h --help:  print the help message for this program.\n");
            printf("-l --long:  print each directory in one line with details.\n");
            printf("-a --all:   print every directory, even they are hidden.\n");
            printf("-s --sort:  sort the results in the alphabetical order.\n");
            break;
        case 'l':
            // printf("Long option.\n");
            arguments[OPT_LONG] = 1;
            // printf("Command_OPT: %d\n", command_opt);
            break;
        case 'a':
            // printf("Show hidden.\n");
            arguments[OPT_ALL] = 1;
            // printf("Command_OPT: %d\n", command_opt);
            break;
        case 's':
            // printf("Sort allowed.\n");
            arguments[OPT_SORT] = 1;
            // printf("Command_OPT: %d\n", command_opt);
            break;
        case '?':
            fprintf(stderr, "Unknown option: %c\n", optopt);
            printf("Use -h or --help to see the help message.\n");
            exit(EXIT_FAILURE);
        default:
            fprintf(stderr, "Error parsing options\n");
            exit(EXIT_FAILURE);
        }
    }

    if(optind == argc){
        // printf("This program %s will print the content of current directory.\n", argv[0]);
        list(NULL, arguments);
    }
    else{
        // printf("non-listed arguments: ");
        // printf("%s ", argv[optind++]);
        // printf("\n");
        list(argv[optind++], arguments);
    }

    return 0;
}

void check_hidden(struct ls_entry *current_entry){
    char e_name[MAX_LEN + 1];
    unsigned long e_size = 0;

    for (int i = 0; current_entry->name[i] != '\0'; i++)
    {
        e_name[i] = current_entry->name[i];
        e_size++;
    }

    if (e_name[0] == '.' || (e_name[0] == '.' && e_name[1] == '.'))
    {
        // debug log
        // printf("Hidden file or parent directories...\n");
        (*current_entry).is_hidden = 1;
    }
}

char additional_dir_type_process(unsigned char c){
    char ret;
    switch (c)
    {
    case 4:
        ret = 'd';
        break;
    case 10:
        ret = 'l';
        break;
    default:
        ret = '-';
        break;
    }
    return ret;
}

int cmp(const void *p1, const void *p2){
    const struct ls_entry *ls_entry_1 = p1;
    const struct ls_entry *ls_entry_2 = p2;

    // place the current directory . first
    if (strcmp(ls_entry_1->name, ".") == 0)
        return -1;
    if (strcmp(ls_entry_2->name, ".") == 0)
        return 1;

    // place the upper directory .. second
    if (strcmp(ls_entry_1->name, "..") == 0)
        return -1;
    if (strcmp(ls_entry_2->name, "..") == 0)
        return 1;

    return strcmp(ls_entry_1->name, ls_entry_2->name);
}

void list(char directory[], int arguments[]){
    DIR *dir_stream;
    struct dirent *dir_read; // hold the "current result of dir stream"
    struct ls_entry *entries = NULL;
    size_t size_of_entries = 0;

    // check the passed argument first
    if (directory != NULL){
        dir_stream = opendir(directory);
    }
    else {
        dir_stream = opendir(".");
    }

    // need to open it first before displaying content?
    // the default behavior or ls is diplaying current "." directory
    // dir_stream = opendir(".");

    // error handler
    if (dir_stream == NULL){
        perror("Error while trying to open directory");
        exit(EXIT_FAILURE);
    }

    while((dir_read = readdir(dir_stream)) != NULL){
        // allocate a (reallocate in fact) space for incoming entry
        entries = realloc(entries, (size_of_entries + 1)*sizeof(struct ls_entry));
        strcpy(entries[size_of_entries].name, dir_read->d_name);
        entries[size_of_entries].size = dir_read->d_reclen;
        entries[size_of_entries].type = additional_dir_type_process(dir_read->d_type);
        check_hidden(&entries[size_of_entries]);
        size_of_entries++;
    }
    // if the command readdir (read directory) from the dir_stream we open is not NULL
    // first, we should grab the basic ones, file size and file name?
    // while ((dir_read = readdir(dir_stream)) != NULL) {
    //     printf("%c\t%s\t%d bytes\n", additional_dir_type_process(dir_read->d_type), dir_read->d_name, dir_read->d_reclen);
    // }

    if(closedir(dir_stream) == -1){
        perror("Cannot close file stream, bugs happened");
        exit(EXIT_FAILURE);
    }

    if(arguments[OPT_SORT] == 1){
        // printf("Sort is selected...\n");
        qsort(entries, size_of_entries, sizeof(struct ls_entry), cmp);
    }
    // sort the whole result array
    if (arguments[OPT_LONG] == 1){
        // printf("Show details...\n");
        for(int i = 0; i < size_of_entries; i++){
            if(arguments[OPT_ALL] == 1){
                // printf("Show hidden files...\n");
                printf("%c\t%s\t%ld bytes\n", entries[i].type, entries[i].name, entries[i].size);
            }
            else{
                if(entries[i].is_hidden == 1)
                    continue;
                printf("%c\t%s\t%ld bytes\n", entries[i].type, entries[i].name, entries[i].size);
            }
        }
    }
    else{
        // normal behavior, print just filename in one line
        for(int i = 0; i < size_of_entries; i++){   
            if (arguments[OPT_ALL] == 1){
                // printf("Show hidden files...\n");
                printf("%s ", entries[i].name);
            }
            else{
                if(entries[i].is_hidden == 1)
                    continue;
                printf("%s ", entries[i].name);
            }
        }
        printf("\n");
    }

    free(entries);

    return;
}
