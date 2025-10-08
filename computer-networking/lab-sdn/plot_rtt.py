import re
import matplotlib.pyplot as plt
import os

# Get the current working directory where the script is running
current_directory = os.getcwd()

# Create the full, dynamic paths for the input and output files
# os.path.join() is used to correctly join paths in a way that works on any OS
input_file_path = os.path.join(current_directory, "h1_to_h2.txt")
output_file_path = os.path.join(current_directory, "rtt_plot_dynamic.png")

# --- Reading the file and generating the plot ---

try:
    # Open and read the input file from the dynamically generated path
    with open(input_file_path, "r") as f:
        data = f.read()

    # Extract the RTT values using regular expressions
    rtt_values = [float(x) for x in re.findall(r"time=(\d+\.\d+)", data)]

    # Create the runtime values (x-axis)
    runtime = range(1, len(rtt_values) + 1)

    # Plotting the data
    plt.plot(runtime, rtt_values, marker='o', linestyle='-')
    plt.title("RTT vs. Runtime (Dynamic Path), from node 1 to node 2")
    plt.xlabel("Runtime (Ping Sequence)")
    plt.ylabel("RTT (ms)")
    plt.grid(True)
    
    # Save the plot to the dynamically generated path
    plt.savefig(output_file_path)
    
    print(f"Plot saved successfully to: {output_file_path}")

except FileNotFoundError:
    print(f"Error: The input file was not found. Please ensure 'h1_to_h2.txt' is in the same directory as the script.")
except Exception as e:
    print(f"An unexpected error occurred: {e}")