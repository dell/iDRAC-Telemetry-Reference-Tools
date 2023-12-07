import _ from 'lodash';
import 'bootstrap/dist/css/bootstrap.min.css';
import 'bootstrap/dist/js/bootstrap.min.js';
import 'jquery'

const feather = require('feather-icons');

feather.replace();
const tls = document.getElementById('tls');
const clientAuth = document.getElementById('clientAuth');
const column1 = document.getElementById('column1');
const column2 = document.getElementById('column2');

tls.addEventListener('change', function () {
    if (tls.checked) {
        column1.style.display = 'block';
    } else {
        column1.style.display = 'none';
    }
});

clientAuth.addEventListener('change', function () {
    if (clientAuth.checked) {
        column2.style.display = 'block';
    } else {
        column2.style.display = 'none';
    }
});

// Functions for various modal operations 
document.getElementById('addServiceModalButton').addEventListener('click', function () {
    $('#serviceModal').modal('show');
})

document.getElementById('closeServiceModal').addEventListener('click', function () {
    $('#serviceModal').modal('hide');
})

document.getElementById('crossServiceModal').addEventListener('click', function () {
    $('#serviceModal').modal('hide');
})

document.getElementById('addServiceButton').addEventListener('click', function () {
    addService();
})

document.getElementById('deleteServiceButton').addEventListener('click', function () {
    deleteService();
})


document.getElementById('uploadCsvModalButton').addEventListener('click', function () {
    $('#csvModal').modal('show');
})

document.getElementById('crossCsvModal').addEventListener('click', function () {
    $('#csvModal').modal('hide');
})

document.getElementById('closeCsvModal').addEventListener('click', function () {
    $('#csvModal').modal('hide');
})

document.getElementById('uploadCsvButton').addEventListener('click', function () {
    uploadFile();
})

document.getElementById('addHECModalButton').addEventListener('click', function () {
    $('#hecModal').modal('show');
})

document.getElementById('crossHECModal').addEventListener('click', function () {
    $('#hecModal').modal('hide');
})

document.getElementById('closeHECModal').addEventListener('click', function () {
    $('#hecModal').modal('hide');
})

document.getElementById('configHECButton').addEventListener('click', function () {
    configHEC();
})

document.getElementById('addKafkaModalButton').addEventListener('click', function () {
    $('#kafkaModal').modal('show');
})

document.getElementById('crossKafkaModal').addEventListener('click', function () {
    $('#kafkaModal').modal('hide');
})

document.getElementById('closeKafkaModal').addEventListener('click', function () {
    $('#kafkaModal').modal('hide');
})

document.getElementById('configKafkaButton').addEventListener('click', function () {
    kafkaConfig();
})


function updateColumnsDisplay() {
    if (tls.checked && clientAuth.checked) {
        column1.style.display = 'block';
        column2.style.display = 'block';
    }
}
// Event listener for modal open
let serviceData;

window.addEventListener('load', function () {
    $.getJSON("/api/v1/Systems", gotServiceList);
    $.getJSON("/api/v1/HttpEventCollector", gotConfigList);
    $.getJSON("/api/v1/KafkaBrokerConnection", gotKafkaConfigList);    
})


// $(function () {
//     $('[data-toggle="tooltip"]').tooltip()
// })

function gotServiceList(data) {
    var tbody = document.getElementById('services');
    serviceData = data;
    var classStr;
    for (var i = 0; i < data.length; i++) {
        classStr = "text-warning"
        switch (data[i].State) {
            case 'Running':
                classStr = "text-success";
                break;
            case 'Stopped':
                classStr = "text-danger";
                break;
            case 'Connection Failed':
                classStr = "text-danger";
                break;
            case 'Telemetry Service Not Found':
                classStr = "text-danger";
                break;
        }

        var serviceRow = tbody.insertRow();

        var hostname = serviceRow.insertCell(0);
        hostname.innerHTML = data[i].Hostname;

        var username = serviceRow.insertCell(1);
        username.innerHTML = data[i].Username;

        var state = serviceRow.insertCell(2);
        state.innerHTML = data[i].State;
        state.classList.add(classStr);

        var lastEvent = serviceRow.insertCell(3);
        lastEvent.innerHTML = new Date(data[i].LastEvent);

        var checkBoxService = serviceRow.insertCell(4);
        var checkbox = document.createElement('input');
        checkbox.classList.add('check');
        checkbox.type = 'checkbox';
        checkbox.id = "checkboxservice-" + i;
        checkbox.addEventListener('click', checkboxClick);
        checkBoxService.appendChild(checkbox);

        // tbody.append('<tr><td>' + data[i].Hostname + '</td><td>' + data[i].Username + '</td><td class="' + classStr + '">' + data[i].State + '</td><td>' + new Date(data[i].LastEvent) + "</td><td><input type='checkbox' id='checkboxservice-" + i + "'' class='check' onclick='checkboxClick(this);' ></td></tr>")
    }
}



function gotConfigList(data) {
    var tbody = $('#hec');
    tbody.append('<tr><td>' + data.url + '</td><td>' + data.key + '</td><td>' + data.index + '</td></tr>')
}

function gotKafkaConfigList(data) {
    var tbody = $('#kafka');
    tbody.append('<tr><td>' + data.kafkaBroker + '</td><td>' + data.kafkaTopic + '</td><td>' + data.tls + '</td><td>' + data.kafkaSkipVerify + '</td><td>' + data.clientAuth + '</td></tr>')
}

function checkboxClick(evt) {
    var checkbox = evt.currentTarget
    let checkboxIndex = parseInt(checkbox.id.split('-')[1])
    if (checkbox.checked) {
        serviceData[checkboxIndex].toDelete = true
    }
    else {
        serviceData[checkboxIndex].toDelete = false
    }
    if ($(".check:checked").length) $("#deleteServiceButton").show();
    else $("#deleteServiceButton").hide();
};

function readFileContents(file, callback) {
    const reader = new FileReader();
    reader.onload = function (event) {
        const contents = event.target.result;
        callback(contents);
    };
    reader.readAsText(file);
}
function isValidIP(ip) {
    // Boolean function to check whether argument is a valid IP using regex
    matches = ip.match('^(?:[0-9]{1,3}\.){3}[0-9]{1,3}$')
    if (matches == null) {
        console.log("IP not valid")
        return false
    } else {
        console.log("IP valid")
        return true
    }
}


async function uploadFile() {
    let formData = new FormData();
    var csvFile = fileupload.files[0];
    var isValidIPFlag = null;
    var isValidfieldsFlag = null;
    var isValidCSVFlag = null;
    if (csvFile) {

        // Check the format of the uploaded CSV file. 3 values in each line
        // and  first value should be a valid IP
        var reader = new FileReader();
        reader.readAsText(csvFile, "UTF-8");
        reader.onload = function (e) {
            var rows = e.target.result.trim().split("\n");
            for (var i = 0; i < rows.length; i++) {
                console.log('row', i)
                var cells = rows[i].trim().split(",");

                //  Check if each row in the CSV file has 3 values
                if (cells.length == 3) {
                    isValidfieldsFlag = true
                    // condition to check whether the IP is correct format
                    isValidIPFlag = isValidIP(cells[0])
                } else {
                    console.log("Row ", i, " should have 3 values")
                    isValidfieldsFlag = false;
                }
            }
            // Uploaded file is invalid if either of the 2 conditions mentioned
            // above (3 values in each line and  first value should be a valid IP)
            // are not satisfied
            isValidCSVFlag = isValidIPFlag && isValidfieldsFlag

            // Send POST request if uploaded file is valid
            if (isValidCSVFlag) {
                console.log("isValidCSV if ", isValidCSVFlag)
                formData.append("file", fileupload.files[0]);
                //  Send POST request with csv file as body 
                fetch('/api/v1/CsvUpload', {
                    method: "POST",
                    body: formData,
                    contentType: 'text/csv'
                }).then((uploadResponse) => {
                    if (!uploadResponse.ok) {
                        throw new Error(uploadResponse.statusText)
                    } else {
                        console.log("Row ", i, " should have 3 values")
                        isValidfieldsFlag = false;
                    }
                }).catch((error) => {
                    alert("A runtime error has occured.")
                });
            } else if (!isValidCSVFlag) {
                // Alert the user if the uploaded file doesn't have the correct format
                console.log("isValidCSV else ", isValidCSVFlag)
                alert("Invalid CSV file format. Please ensure that the first value in every row "
                    + "is a valid IP and that there are 3 values in each row of the csv file (IP, username, password)")
            }
        }
    }

}

function addService() {
    var arr = $('#newService').serializeArray();
    var obj = {};
    for (var i = 0; i < arr.length; i++) {
        obj[arr[i].name] = arr[i].value;
    }
    $.ajax({
        url: '/api/v1/Systems',
        method: 'POST',
        data: JSON.stringify(obj),
        contentType: 'application/json',
        dataType: 'json',
        complete: addDone
    })
}
function deleteService() {
    var hostnames = []
    var h = 0
    for (var i = 0; i < serviceData.length; i++) {
        if (serviceData[i].toDelete == true) {
            hostnames[h++] = serviceData[i].Hostname
        }
    }
    $.ajax({
        url: '/api/v1/Delete',
        method: 'POST',
        data: JSON.stringify({ Hostname: hostnames }),
        contentType: 'application/json',
        dataType: 'json',
        complete: deleteDone
    })
}

function configHEC() {
    var arr = $('#newHEC').serializeArray();
    var obj = {};
    for (var i = 0; i < arr.length; i++) {
        obj[arr[i].name] = arr[i].value;
    }
    $.ajax({
        url: '/api/v1/HecConfig',
        method: 'POST',
        data: JSON.stringify(obj),
        contentType: 'application/json',
        dataType: 'json',
        complete: addhec
    })
}

// Getting the data from kafka model form and post an ajax request
function kafkaConfig() {
    var form = document.getElementById('newKafka');
    var formData = new FormData(form);

    formData.forEach(function (value, key) {
        console.log(key, value);

        // For file inputs
        if (key.startsWith("file")) {
            console.log(key, value.name); // Logs the name of the uploaded file
        }
    });


    var obj = {
        kafkaBroker: $("#Broker").val(),
        kafkaTopic: $("#Topic").val()
    };

    if ($("#tls").is(":checked")) {
        obj.tls = "true";
        obj.kafkaSkipVerify = $("#kafkaskipverify").is(":checked") ? "true" : "false";
        const file1 = $('#fileupload1')[0].files[0];
        if (file1) {
            console.log("file1 ", file1);
            readFileContents(file1, function (contents) {
                obj.kafkaCACert = contents
            });
            console.log("kafkaCACert File:", file1.name);
        }
    }



    if ($("#clientAuth").is(":checked")) {
        obj.clientAuth = "true";
        const file2 = $('#fileupload2')[0].files[0];
        if (file2) {
            readFileContents(file2, function (contents) {
                obj.kafkaClientCert = contents
            });
            console.log("kafkaClientCert File:", file2.name);
        }
        const file3 = $('#fileupload3')[0].files[0];
        if (file3) {
            readFileContents(file3, function (contents) {
                obj.kafkaClientKey = contents
            });
            console.log("kafkaClientKey File:", file3.name);
        }
    }

    // sleep 3s
    (async () => {
        if (tls) {
            await new Promise(done => setTimeout(() => done(), 3000));
        }

        //Send the AJAX request
        console.log("printing full object", JSON.stringify(obj))
        console.log(JSON.stringify("POST REQUEST DATA" + new FormData($("#newKafka")[0])));

        $.ajax({
            url: '/api/v1/KafkaConfig',
            method: 'POST',
            data: JSON.stringify(obj),
            contentType: 'application/json',
            dataType: 'json',
            complete: kafkaDone
        });

    })();
}
function addDone(jqXHR) {
    
    if (jqXHR.status != 200) {
        alert("Failed to add new service!");
    }
    else {
        window.location.reload(true);
    }
}

function addhec(jqXHR) {
    if (jqXHR.status != 200) {
        alert("Failed to config hec!");
    }
    else {
        window.location.reload();
    }
}
function deleteDone(jqXHR) {
    if (jqXHR.status != 200) {
        alert("Failed to delete new service!");
    }
    else {
        window.location.reload();
    }
}

function kafkaDone(jqXHR) {
    if (jqXHR.status != 200) {
        alert("Failed to add kafka config!");
    }
    else {
        window.location.reload();
        form = document.getElementById('newKafka');
        form.reset();
    }
}
