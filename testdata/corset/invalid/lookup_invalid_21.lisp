;;error:4:18-21:empty source columns
;;error:4:24-27:empty target columns
(defcolumns (X :i16) (Y :i16))
(deflookup test ((_)) ((_)))
