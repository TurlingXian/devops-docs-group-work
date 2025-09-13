#include <dirent.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#define MAX_LEN 256

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
char additional_dir_type_process(unsigned char c)
{
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

/**
 * This function is to handle the arguments passed to the program. Both long and short arguments are accepted.
 * @param: argc, argv
 * @return: print the help message or error is the input is not correct. Print the
 */
int parse_arguments(int argc, char *argv[])
{
    int opt_result = 0; // default is 0, no option was passed and the command will print the short list.
    // debug message
    printf("The number of the arguments passed: %d\n", argc);
    for (int i = 0; i < argc; i++)
    {
        printf("Argument %d: %s\n", i, argv[i]);
    }

    // begin to parse the arguments
    int opt;
    // int flag = 0;
    while ((opt = getopt(argc, argv, "lah")) != -1)
    {
        switch (opt)
        {
        case 'h':
            printf("This command is used to list information about the directory, default is listing the current directory or the directory you passed to this command.\n");
            printf("Usage: %s [-l] [-a] [-h]\n", argv[0]);
            printf("  -l           Use a long listing format\n");
            printf("  -a           Include directory entries whose names begin with a dot (.) - or a hidden one.\n");
            printf("  -h           Display this help message\n");
            exit(EXIT_SUCCESS);
        case 'l':
            // long listing format
            // debug message
            // printf("Long listing format...\n");
            opt_result += 1;
            break;
        case 'a':
            // include the hidden file and the parent directory
            // debug message
            // printf("Including hidden files...\n");
            opt_result += 2;
            break;
        case '?':
            fprintf(stderr, "Unknown option: %c\n", optopt);
            printf("Use -h to see the help message.\n");
            exit(EXIT_FAILURE);
        default:
            fprintf(stderr, "Error parsing options\n");
            exit(EXIT_FAILURE);
        }
    }

    // scan all for all arguments passed
    // if (optind < argc)
    // {
    //     printf("Positional arguments:\n");
    //     for (int i = optind; i < argc; i++)
    //     {
    //         printf(" %s\n", argv[i]);
    //     }
    // }

    // debug message
    // printf("The result of parsing the arguments: %d\n", opt_result);
    return opt_result;
}

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
 * @param: Two entry (ls_entry).
 *
 * @return: indicate which comes first.
 */
int cmp(const void *p1, const void *p2)
{
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

/**
 * Handler hidden file or directories.
 * Using the d_name to catch.
 * @param: struct ls_entry
 *
 * @return: modifies the property of ls_entry
 */
void check_hidden(struct ls_entry *current_entry)
{
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

int main(int arc, char *argv[])
{
    int parse_result = parse_arguments(arc, argv);
    // printf("The result of parsing the arguments: %d\n", parse_result);
    // // a pointer points to a directory
    // DIR *dir_stream;
    // struct dirent *dir_read; // hold the "current result of dir stream"
    // struct ls_entry *entries = NULL;
    // size_t size_of_entries = 0;

    // // need to open it first before displaying content?
    // // the default behavior or ls is diplaying current "." directory
    // dir_stream = opendir(".");

    // // error handler
    // if (dir_stream == NULL){
    //     perror("Error while trying to open directory");
    //     return -1;
    // }

    // while((dir_read = readdir(dir_stream)) != NULL){
    //     // allocate a (reallocate in fact) space for incoming entry
    //     entries = realloc(entries, (size_of_entries + 1)*sizeof(struct ls_entry));
    //     strcpy(entries[size_of_entries].name, dir_read->d_name);
    //     entries[size_of_entries].size = dir_read->d_reclen;
    //     entries[size_of_entries].type = additional_dir_type_process(dir_read->d_type);
    //     check_hidden(&entries[size_of_entries]);
    //     size_of_entries++;
    // }
    // // if the command readdir (read directory) from the dir_stream we open is not NULL
    // // first, we should grab the basic ones, file size and file name?
    // // while ((dir_read = readdir(dir_stream)) != NULL) {
    // //     printf("%c\t%s\t%d bytes\n", additional_dir_type_process(dir_read->d_type), dir_read->d_name, dir_read->d_reclen);
    // // }

    // if(closedir(dir_stream) == -1){
    //     perror("Cannot close file stream, bugs happened");
    //     return -1;
    // }

    // // sort the whole result array
    // qsort(entries, size_of_entries, sizeof(struct ls_entry), cmp);

    // // normally, we should hide the "." and ".." or any directory begin with .
    // // the most primitive of using "hidden"
    // for(int i = 0; i < size_of_entries; i++){
    //     if (entries[i].is_hidden != 1){
    //         // printf("Will be printed...\n");
    //         printf("%c\t%s\t\t\t%ld bytes\n", entries[i].type, entries[i].name, entries[i].size);
    //     }
    // }

    // free(entries);

    return 0;
}