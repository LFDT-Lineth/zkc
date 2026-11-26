(defcolumns (X :i16) (Y :i16) (Z :i16) (one :i1 :constant 1))
;; fragmented lookup with two targets
(deflookup test ((one X)) ((one Y) (one Z)))
